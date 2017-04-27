/*
* amqp.go
*
* the amqp functions takes care of communication via the AMQP protocol
*
* Two threads can be used for receiving and sending AMQP messages, respectively: AmqpReceiver and AmqpSender
 */

package hornet

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/kardianos/osext"
	"github.com/pborman/uuid"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"github.com/ugorji/go/codec"

	"github.com/project8/hornet/gogitver"
)

type SenderInfo struct {
	Package  string
	Exe      string
	Version  string
	Commit   string
	Hostname string
	Username string
}

type P8Message struct {
	Target    []string
	Encoding  string
	CorrId    string
	MsgType   MsgCodeT
	MsgOp     MsgCodeT
	RetCode   MsgCodeT
	RetMsg    string
	TimeStamp string
	SenderInfo
	Payload   interface{}
	ReplyChan chan P8Message
}

// Globally-accessible message-sending queue
var SendMessageQueue = make(chan P8Message, 100)

// Separator for the routing key/target parts
var TargetSeparator string = "."

// Value to confirm that the AMQP sender routine has started
var AmqpSenderIsActive bool = false

// Value to confirm that the AMQP receiver routine has started
var AmqpReceiverIsActive bool = false

// Map of correlation ID to channel to handle the reply message
var replyMap map[string]chan P8Message

var MasterSenderInfo SenderInfo

func fillMasterSenderInfo() (e error) {
	MasterSenderInfo.Package = "hornet"
	MasterSenderInfo.Exe, e = osext.Executable()
	if e != nil {
		Log.Errorf("Error while getting the executable:\n\t%v", e)
	}
	MasterSenderInfo.Version = gogitver.Tag()
	MasterSenderInfo.Commit = gogitver.Git()
	MasterSenderInfo.Hostname, e = os.Hostname()
	if e != nil {
		Log.Errorf("Error while getting the hostname:\n\t%v", e)
	}
	user, userErr := user.Current()
	e = userErr
	if e != nil {
		Log.Errorf("Error while getting the username:\n\t%v", e)
	} else {
		MasterSenderInfo.Username = user.Username
	}
	return
}

// ValidateAmqpConfig checks the sanity of the amqp section of a configuration.
// It makes the following guarantees
//   1) The broker setting is present
//   2) If the receiver is present and active, then the queue and exchange are set.
func ValidateAmqpConfig() (e error) {
	if viper.IsSet("amqp.active") == false {
		e = errors.New("amqp.active is not set")
		Log.Error(e.Error())
	}
	if viper.GetBool("amqp.active") == false {
		return
	}

	if viper.IsSet("amqp.queue") == false {
		e = errors.New("Queue name is not set (amqp.queue)")
		Log.Error(e.Error())
	}

	if viper.IsSet("amqp.broker") == false || viper.IsSet("amqp.exchange") == false {
		e = errors.New("AMQP sender/receiver cannot be used without the broker and exchange being set")
		Log.Error(e.Error())
	}

	return
}

func StartAmqp(ctrlQueue, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) (e error) {
	if configErr := ValidateAmqpConfig(); configErr != nil {
		Log.Criticalf("Error in the AMQP configuration: %s", configErr.Error())
		reqQueue <- ThreadCannotContinue
		return
	}

	if viper.GetBool("amqp.active") == false {
		Log.Notice("AMQP is inactive")
		return
	}

	Log.Info("Starting AMQP components")

	e = fillMasterSenderInfo()
	if e != nil {
		Log.Critical("Cannot start AMQP; failed to get master sender info")
		return
	}

	Log.Info("Starting AMQP receiver")
	poolCount.Add(1)
	threadCountQueue <- 1
	go AmqpReceiver(ctrlQueue, reqQueue, poolCount)

	Log.Info("Starting AMQP sender")
	poolCount.Add(1)
	threadCountQueue <- 1
	go AmqpSender(ctrlQueue, reqQueue, poolCount)

	return
}

// AmqpReceiver is a goroutine for receiving and handling AMQP messages
func AmqpReceiver(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	// decrement the wg counter at the end
	defer poolCount.Done()
	defer Log.Info("[AMQP receiver is finished.")

	// Connect to the AMQP broker
	// Deferred command: close the connection
	brokerAddress := viper.GetString("amqp.broker")
	if viper.GetBool("amqp.use-auth") {
		if Authenticators.Amqp.Available == false {
			Log.Critical("AMQP authentication is not available")
			reqQueue <- StopExecution
			return
		}
		brokerAddress = Authenticators.Amqp.Username + ":" + Authenticators.Amqp.Password + "@" + brokerAddress
	}
	brokerAddress = "amqp://" + brokerAddress
	if viper.IsSet("amqp.port") {
		brokerAddress = brokerAddress + ":" + viper.GetString("amqp.port")
	}
	connection, receiveErr := amqp.Dial(brokerAddress)
	if receiveErr != nil {
		Log.Criticalf("Unable to connect to the AMQP broker at (%s) for receiving:\n\t%v", brokerAddress, receiveErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer connection.Close()

	// Create the channel object that represents the connection to the broker
	// Deferred command: close the channel
	channel, chanErr := connection.Channel()
	if chanErr != nil {
		Log.Criticalf("Unable to get the AMQP channel:\n\t%v", chanErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer channel.Close()

	// Create the exchange if it doesn't already exist
	exchangeName := viper.GetString("amqp.exchange")
	exchDeclErr := channel.ExchangeDeclare(exchangeName, "topic", false, false, false, false, nil)
	if exchDeclErr != nil {
		Log.Criticalf("Unable to declare exchange <%s>:\n\t%v", exchangeName, exchDeclErr.Error())
		reqQueue <- StopExecution
		return
	}

	// Declare the "hornet" queue
	// Deferred command: delete the "hornet" queue
	queueName := viper.GetString("amqp.queue")
	_, queueDeclErr := channel.QueueDeclare(queueName, false, true, true, false, nil)
	if queueDeclErr != nil {
		Log.Criticalf("Unable to declare queue <%s>:\n\t%v", queueName, queueDeclErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer func() {
		if _, err := channel.QueueDelete(queueName, false, false, false); err != nil {
			Log.Errorf("Error while deleting queue:\n\t%v", err)
		}
	}()

	// Bind the "hornet" queue to the exchange, and subscribe it to all routing keys that start with "hornet"
	// Deferred command: unbind the "hornet" queue from the exchange
	queueBindErr := channel.QueueBind(queueName, queueName+".#", exchangeName, false, nil)
	if queueBindErr != nil {
		Log.Criticalf("Unable to bind queue <%s> to exchange <%s>:\n\t%v", queueName, exchangeName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer func() {
		if err := channel.QueueUnbind(queueName, queueName+".#", exchangeName, nil); err != nil {
			Log.Errorf("Error while unbinding queue:\n\t%v", err)
		}
	}()

	// Start consuming messages on the queue
	// Channel::Cancel is not executed as a deferred command, because consuming will be stopped by Channel.Close
	messageQueue, consumeErr := channel.Consume(queueName, "", false, true, true, false, nil)
	if consumeErr != nil {
		Log.Criticalf("Unable start consuming from queue <%s>:\n\t%v", queueName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}

	Log.Info("AMQP Receiver started successfully")
	AmqpReceiverIsActive = true
	defer func() { AmqpReceiverIsActive = false }()

amqpLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg, queueOk := <-ctrlQueue:
			if !queueOk {
				Log.Error("Control queue has closed unexpectedly")
				break amqpLoop
			}
			if controlMsg == StopExecution {
				Log.Info("AMQP receiver stopping on interrupt.")
				break amqpLoop
			}
		// process any AMQP messages that are received
		case message, queueOk := <-messageQueue:
			if !queueOk {
				Log.Error("AMQP message queue has closed unexpectedly")
				reqQueue <- StopExecution
				break amqpLoop
			}

			// Send an acknowledgement to the broker
			if ackErr := message.Ack(false); ackErr != nil {
				Log.Error("Unable to acknowledge AMQP message")
			}

			// Decode the body of the message
			//log.Printf("[amqp receiver] Received message with encoding %s", message.ContentEncoding)
			var body map[string]interface{}
			switch message.ContentEncoding {
			case "application/json":
				//log.Printf("this is a json message")
				handle := new(codec.JsonHandle)
				decoder := codec.NewDecoderBytes(message.Body, handle)
				jsonErr := decoder.Decode(&body)
				if jsonErr != nil {
					Log.Error("Unable to decode JSON-encoded message:\n\t%v", jsonErr)
					continue amqpLoop
				}
			case "application/msgpack":
				//log.Printf("this is a msgpack message")
				handle := new(codec.MsgpackHandle)
				decoder := codec.NewDecoderBytes(message.Body, handle)
				msgpackErr := decoder.Decode(&body)
				if msgpackErr != nil {
					Log.Error("Unable to decode msgpack-encoded message:\n\t%v", msgpackErr)
					continue amqpLoop
				}
			default:
				Log.Error("Message content encoding is not understood: %s", message.ContentEncoding)
				continue amqpLoop
			}
			//log.Printf("[amqp receiver] Message body:\n\t%v", body)

			// Message contents validation
			// required elements: msgtype, timestamp, sender_info
			msgTypeIfc, msgtypePresent := body["msgtype"]
			timestampIfc, timestampPresent := body["timestamp"]
			senderInfoIfc, senderInfoPresent := body["sender_info"]
			if msgtypePresent && timestampPresent && senderInfoPresent == false {
				Log.Error("Message is missing a required element:\n\tmsgtype: %v\n\ttimestamp: %v\n\tsender_info: %v", msgtypePresent, timestampPresent, senderInfoPresent)
				continue amqpLoop
			}
			msgType := ConvertToMsgCode(msgTypeIfc)

			// Translate the body of the message into a P8Message object
			senderInfo := senderInfoIfc.(map[interface{}]interface{})
			p8Message := P8Message{
				Encoding:  message.ContentEncoding,
				CorrId:    message.CorrelationId,
				MsgType:   msgType,
				TimeStamp: ConvertToString(timestampIfc),
				SenderInfo: SenderInfo{
					Package:  ConvertToString(senderInfo["package"]),
					Exe:      ConvertToString(senderInfo["exe"]),
					Version:  ConvertToString(senderInfo["version"]),
					Commit:   ConvertToString(senderInfo["commit"]),
					Hostname: ConvertToString(senderInfo["hostname"]),
					Username: ConvertToString(senderInfo["username"]),
				},
			}

			if payloadIfc, hasPayload := body["payload"]; hasPayload {
				p8Message.Payload = payloadIfc
			}

			// validation for certain types of messages
			switch msgType {
			case MTReply:
				if retcodeIfc, retcodePresent := body["retcode"]; !retcodePresent {
					Log.Error("Message is missing a required element:\n\tretcode: %v", retcodePresent)
					continue amqpLoop
				} else {
					p8Message.RetCode = ConvertToMsgCode(retcodeIfc)
				}
				if retmsgIfc, retmsgPresent := body["return_msg"]; !retmsgPresent {
					Log.Warning("Message is missing a required element:\n\treturn_msg: %v", retmsgPresent)
					//continue amqpLoop
				} else {
					p8Message.RetMsg = ConvertToString(retmsgIfc)
				}
			case MTRequest:

				if msgopIfc, msgopPresent := body["msgop"]; !msgopPresent {
					Log.Error("Request message is missing a required element:\n\tmsgop: %v", msgopPresent)
					continue amqpLoop
				} else {
					p8Message.MsgOp = ConvertToMsgCode(msgopIfc)
				}
			case MTAlert:
				Log.Error("Cannot handle Alert messages")
			case MTInfo:
				Log.Error("Cannot handle Info messages")
			default:
				Log.Error("Unknown message type: %v", msgType)
			}

			routingKeyParts := strings.Split(message.RoutingKey, TargetSeparator)
			if len(routingKeyParts) > 1 {
				p8Message.Target = routingKeyParts[1:len(routingKeyParts)]
			}

			//log.Printf("[amqp receiver] Message:\n\t%v", p8Message)

			// Handle with the message according to the message type
			switch msgType {
			case MTReply:
				Log.Info("Received reply message: (%d) %s", p8Message.RetCode, p8Message.RetMsg)
				if replyHandlerChan, canReply := replyMap[message.CorrelationId]; canReply {
					replyHandlerChan <- p8Message
				}
			case MTRequest:
				// Handle with the request message according to the target
				if len(p8Message.Target) == 0 {
					Log.Error("No Hornet target provided")
				} else {
					switch p8Message.Target[0] {
					case "quit-hornet":
						reqQueue <- StopExecution
					case "print-message":
						Log.Notice("Message received for printing:")
						Log.Notice("\tEncoding: %v", p8Message.Encoding)
						Log.Notice("\tCorrelation ID: %v", p8Message.CorrId)
						Log.Notice("\tMessage Type: %v", p8Message.MsgType)
						Log.Notice("\tTimestamp: %v", p8Message.TimeStamp)
						Log.Notice("\tSenderInfo:")
						Log.Notice("\t\tPackage: %v", p8Message.SenderInfo.Package)
						Log.Notice("\t\tExe: %v", p8Message.SenderInfo.Exe)
						Log.Notice("\t\tVersion: %v", p8Message.SenderInfo.Version)
						Log.Notice("\t\tCommit: %v", p8Message.SenderInfo.Commit)
						Log.Notice("\t\tHostname: %v", p8Message.SenderInfo.Hostname)
						Log.Notice("\t\tUsername: %v", p8Message.SenderInfo.Username)
						Log.Notice("\tPayload:")
						switch typedPayload := p8Message.Payload.(type) {
						case string:
							Log.Notice("\t\t%s", typedPayload)
						case map[interface{}]interface{}:
							for key, value := range p8Message.Payload.(map[interface{}]interface{}) {
								switch typedValue := value.(type) {
								case []byte:
									Log.Notice("\t\t%s (byte sl): %v", key.(string), string(typedValue))
								case [][]byte:
									//log.Printf("\t\t%s (byte sl sl): %v", key.(string), [][]string(typedValue))
									sliceString := "["
									for _, byteSlice := range typedValue {
										_ = fmt.Sprintf(sliceString, "%s, %s", sliceString, string(byteSlice))
									}
									_ = fmt.Sprintf(sliceString, "%s]", sliceString)
									Log.Notice("\t\t%s (byte sl sl): %s", key.(string), sliceString)
								case rune, bool, int, uint, float32, float64, complex64, complex128, string:
									Log.Notice("\t\t%s (type): %v", key.(string), typedValue)
								case []rune, []bool, []int, []uint, []float32, []float64, []complex64, []complex128, []string:
									Log.Notice("\t\t%s (array): %v", key.(string), typedValue)
								default:
									Log.Notice("\t\t%s (default): %v", key.(string), value)
								}
							}
						default:
							Log.Warning("Unknown payload type")
						}
					default:
						Log.Error("Unknown hornet target for request messages: %v", p8Message.Target)
					}
				}
			}
		} // end select block
	} // end for loop
}

// AmqpSender is a goroutine responsible for sending AMQP messages received on a channel
func AmqpSender(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	// decrement the wg counter at the end
	defer poolCount.Done()
	defer Log.Info("AMQP sender is finished.")

	// Connect to the AMQP broker
	// Deferred command: close the connection
	brokerAddress := viper.GetString("amqp.broker")
	if viper.GetBool("amqp.use-auth") {
		if Authenticators.Amqp.Available == false {
			Log.Critical("AMQP authentication is not available")
			reqQueue <- StopExecution
			return
		}
		brokerAddress = Authenticators.Amqp.Username + ":" + Authenticators.Amqp.Password + "@" + brokerAddress
	}
	brokerAddress = "amqp://" + brokerAddress
	if viper.IsSet("amqp.port") {
		brokerAddress = brokerAddress + ":" + viper.GetString("amqp.port")
	}
	connection, receiveErr := amqp.Dial(brokerAddress)
	if receiveErr != nil {
		Log.Critical("Unable to connect to the AMQP broker at (%s) for receiving:\n\t%v", brokerAddress, receiveErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer connection.Close()

	// Create the channel object that represents the connection to the broker
	// Deferred command: close the channel
	channel, chanErr := connection.Channel()
	if chanErr != nil {
		Log.Critical("Unable to get the AMQP channel:\n\t%v", chanErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer channel.Close()

	exchangeName := viper.GetString("amqp.exchange")

	replyTo := viper.GetString("amqp.queue")

	Log.Info("AMQP sender started successfully")
	AmqpSenderIsActive = true
	defer func() { AmqpSenderIsActive = false }()

amqpLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg, queueOk := <-ctrlQueue:
			if !queueOk {
				Log.Error("Control queue closed unexpectedly")
				break amqpLoop
			}
			if controlMsg == StopExecution {
				Log.Info("AMQP sender stopping on interrupt.")
				break amqpLoop
			}
		// process any message reuqests received on the send-messsage queue
		case p8Message, queueOk := <-SendMessageQueue:
			if !queueOk {
				Log.Error("Send-message queue has closed")
				reqQueue <- StopExecution
				break amqpLoop
			}

			// Translate the request into a map that can be encoded for transmission
			var senderInfo = map[string]interface{}{
				"package":  p8Message.SenderInfo.Package,
				"exe":      p8Message.SenderInfo.Exe,
				"version":  p8Message.SenderInfo.Version,
				"commit":   p8Message.SenderInfo.Commit,
				"hostname": p8Message.SenderInfo.Hostname,
				"username": p8Message.SenderInfo.Username,
			}
			var body = map[string]interface{}{
				"msgtype":     p8Message.MsgType,
				"msgop":       p8Message.MsgOp,
				"retcode":     p8Message.RetCode,
				"return_msg":  p8Message.RetMsg,
				"timestamp":   p8Message.TimeStamp,
				"sender_info": senderInfo,
				"payload":     p8Message.Payload,
			}

			// Get the UUID for the correlation ID
			correlationId := p8Message.CorrId
			if p8Message.CorrId == "" {
				correlationId = uuid.New()
			}

			// if a reply is requested (as indicated by a non-nil reply channel), add the channel to the reply map
			if p8Message.ReplyChan != nil {
				replyMap[correlationId] = p8Message.ReplyChan
			}

			//log.Printf("[amqp sender] Received message to send:\n\t%v", body)
			bodyNBytes := unsafe.Sizeof(p8Message)
			//log.Printf("[amqp sender] Message size in bytes: %d", bodyNBytes)

			var message = amqp.Publishing{
				ContentEncoding: p8Message.Encoding,
				Body:            make([]byte, 0, bodyNBytes),
				ReplyTo:         replyTo,
				CorrelationId:   correlationId,
			}
			// Encode the message body for transmission
			switch p8Message.Encoding {
			case "application/json":
				//log.Printf("this will be a json message")
				handle := new(codec.JsonHandle)
				encoder := codec.NewEncoderBytes(&(message.Body), handle)
				jsonErr := encoder.Encode(&body)
				if jsonErr != nil {
					Log.Error("Unable to decode JSON-encoded message:\n\t%v", jsonErr)
					continue amqpLoop
				}
			case "application/msgpack":
				//log.Printf("this will be a msgpack message")
				handle := new(codec.MsgpackHandle)
				encoder := codec.NewEncoderBytes(&(message.Body), handle)
				msgpackErr := encoder.Encode(&body)
				if msgpackErr != nil {
					Log.Error("Unable to decode msgpack-encoded message:\n\t%v", msgpackErr)
					continue amqpLoop
				}
			default:
				Log.Error("Message content cannot be encoded with type <%s>", p8Message.Encoding)
				continue amqpLoop
			}

			routingKey := strings.Join(p8Message.Target, TargetSeparator)

			//log.Printf("[amqp sender] Encoded message:\n\t%v", message)
			Log.Debug("Sending message to routing key <%s>", routingKey)

			// Publish!
			pubErr := channel.Publish(exchangeName, routingKey, false, false, message)
			if pubErr != nil {
				Log.Error("Error while sending message:\n\t%v", pubErr)
			}

		} // end select block
	} // end for loop
}

// PrepareRequest sets up most of the fields in a P8Message request object.
// The payload is not set here.
func PrepareRequest(target []string, encoding string, msgOp MsgCodeT, replyChan chan P8Message) (p8Message P8Message) {
	p8Message = P8Message{
		Target:     target,
		Encoding:   encoding,
		MsgType:    MTRequest,
		MsgOp:      msgOp,
		TimeStamp:  time.Now().UTC().Format(TimeFormat),
		SenderInfo: MasterSenderInfo,
		ReplyChan:  replyChan,
	}
	return
}

// PrepareReply sets up most of the fields in a P8Message reply object.
// The payload is not set here.
func PrepareReply(target []string, encoding string, corrId string, retCode MsgCodeT, retMsg string, replyChan chan P8Message) (p8Message P8Message) {
	p8Message = P8Message{
		Target:     target,
		Encoding:   encoding,
		CorrId:     corrId,
		MsgType:    MTReply,
		RetCode:    retCode,
		RetMsg:     retMsg,
		TimeStamp:  time.Now().UTC().Format(TimeFormat),
		SenderInfo: MasterSenderInfo,
		ReplyChan:  replyChan,
	}
	return
}
