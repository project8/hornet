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
	"log"
	"os"
	//"os/exec"
	"os/user"
	//"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/streadway/amqp"
	"github.com/ugorji/go/codec"
	"github.com/kardianos/osext"
	"github.com/spf13/viper"

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
    Target     []string
	Encoding   string
	MsgTypeVal MsgType
	MsgOpVal   MsgOp
	TimeStamp  string
	SenderInfo
	Payload    interface{}
}

// Globally-accessible message-sending queue
var SendMessageQueue = make(chan P8Message, 100)

// Separator for the routing key/target parts
var TargetSeparator string = "."

// Value to confirm that the AMQP sender routine has started
var AmqpSenderIsActive bool = false
// Value to confirm that the AMQP receiver routine has started
var AmqpReceiverIsActive bool = false

var MasterSenderInfo SenderInfo
func fillMasterSenderInfo() (e error) {
	MasterSenderInfo.Package = "hornet"
	MasterSenderInfo.Exe, e = osext.Executable()
	if e != nil {
		log.Printf("[amqp] Error in getting the executable:\n\t%v", e)
	}
	MasterSenderInfo.Version = gogitver.Tag()
	MasterSenderInfo.Commit = gogitver.Git()
	MasterSenderInfo.Hostname, e = os.Hostname()
	if e != nil {
		log.Printf("[amqp] Error in getting the hostname:\n\t%v", e)
	}
	user, userErr := user.Current()
	e = userErr
	if e != nil {
		log.Printf("[amqp] Error in getting the username:\n\t%v", e)
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
	// to see if we need to check on the broker and exchange
	requireAmqp := false	

	if viper.IsSet("amqp.recever") && viper.GetBool("amqp.receiver.active") {
		requireAmqp = true
		if viper.IsSet("amqp.receiver.queue") == false {
			e = errors.New("[amqp] Receiver section is missing the queue")
			log.Print(e.Error())
		}
		
	}

	if viper.IsSet("amqp.sender") && viper.GetBool("amqp.sender.active") {
		requireAmqp = true
		// nothing really to check here
	}

	if requireAmqp && (viper.IsSet("amqp.broker") == false || viper.IsSet("amqp.exchange") == false) {
		e = errors.New("[amqp] AMQP sender/receiver cannot be used without the broker and exchange being set")
		log.Print(e.Error())
	}



	return
}

func StartAmqp(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) (e error) {
	log.Print("[amqp] Starting AMQP components")

	e = fillMasterSenderInfo()
	if e != nil {
		log.Printf("[amqp] Cannot start AMQP; failed to get master sender info")
		return
	}

	if viper.IsSet("amqp.receiver") && viper.GetBool("amqp.receiver.active") {
		log.Print("[amqp] Starting AMQP receiver")
		poolCount.Add(1)
		threadCountQueue <- 1
		go AmqpReceiver(ctrlQueue, reqQueue, poolCount)
	}

	if viper.IsSet("amqp.sender") && viper.GetBool("amqp.sender.active") {
		log.Print("[amqp] Starting AMQP sender")
		poolCount.Add(1)
		threadCountQueue <- 1
		go AmqpSender(ctrlQueue, reqQueue, poolCount)
	}

	return
}

// AmqpReceiver is a goroutine for receiving and handling AMQP messages
func AmqpReceiver(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	// decrement the wg counter at the end
	defer poolCount.Done()

	// Connect to the AMQP broker
	// Deferred command: close the connection
	brokerAddress := viper.GetString("amqp.broker")
	if strings.HasPrefix(brokerAddress, "amqp://") == false {
		brokerAddress = "amqp://" + brokerAddress
	}
	connection, receiveErr := amqp.Dial(brokerAddress)
	if receiveErr != nil {
		log.Printf("[amqp receiver] Unable to connect to the AMQP broker at (%s) for receiving:\n\t%v", brokerAddress, receiveErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer connection.Close()

	// Create the channel object that represents the connection to the broker
	// Deferred command: close the channel
	channel, chanErr := connection.Channel()
	if chanErr != nil {
		log.Printf("[amqp receiver] Unable to get the AMQP channel:\n\t%v", chanErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer channel.Close()

	// Create the exchange if it doesn't already exist
	exchangeName := viper.GetString("amqp.exchange")
	exchDeclErr := channel.ExchangeDeclare(exchangeName, "topic", false, false, false, false, nil)
	if exchDeclErr != nil {
		log.Printf("[amqp receiver] Unable to declare exchange <%s>:\n\t%v", exchangeName, exchDeclErr.Error())
		reqQueue <- StopExecution
		return
	}

	// Declare the "hornet" queue
	// Deferred command: delete the "hornet" queue
	queueName := viper.GetString("amqp.receiver.queue")
	_, queueDeclErr := channel.QueueDeclare(queueName, false, true, true, false, nil)
	if queueDeclErr != nil {
		log.Printf("[amqp receiver] Unable to declare queue <%s>:\n\t%v", queueName, queueDeclErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer func() {
		if _, err := channel.QueueDelete(queueName, false, false, false); err != nil {
			log.Printf("[amqp receiver] Error while deleting queue:\n\t%v", err)
		}
	}()

	// Bind the "hornet" queue to the exchange, and subscribe it to all routing keys that start with "hornet"
	// Deferred command: unbind the "hornet" queue from the exchange
	queueBindErr := channel.QueueBind(queueName, queueName+".#", exchangeName, false, nil)
	if queueBindErr != nil {
		log.Printf("[amqp receiver] Unable to bind queue <%s> to exchange <%s>:\n\t%v", queueName, exchangeName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer func() {
		if err := channel.QueueUnbind(queueName, queueName+".#", exchangeName, nil); err != nil {
			log.Printf("[amqp receiver] Error while unbinding queue:\n\t%v", err)
		}
	}()

	// Start consuming messages on the queue
	// Channel::Cancel is not executed as a deferred command, because consuming will be stopped by Channel.Close
	messageQueue, consumeErr := channel.Consume(queueName, "", false, true, true, false, nil)
	if consumeErr != nil {
		log.Printf("[amqp receiver] Unable start consuming from queue <%s>:\n\t%v", queueName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}

	log.Print("[amqp receiver] started successfully")
	AmqpReceiverIsActive = true
	defer func() {AmqpReceiverIsActive = false}()

amqpLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-ctrlQueue:
			if controlMsg == StopExecution {
				log.Print("[amqp receiver] stopping on interrupt.")
				break amqpLoop
			}
		// process any AMQP messages that are received
		case message := <-messageQueue:
			// Send an acknowledgement to the broker
			message.Ack(false)

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
					log.Printf("[amqp receiver] Unable to decode JSON-encoded message:\n\t%v", jsonErr)
					continue amqpLoop
				}
			case "application/msgpack":
				//log.Printf("this is a msgpack message")
				handle := new(codec.MsgpackHandle)
				decoder := codec.NewDecoderBytes(message.Body, handle)
				msgpackErr := decoder.Decode(&body)
				if msgpackErr != nil {
					log.Printf("[amqp receiver] Unable to decode msgpack-encoded message:\n\t%v", msgpackErr)
					continue amqpLoop
				}
			default:
				log.Printf("[amqp receiver] Message content encoding is not understood: %s", message.ContentEncoding)
				continue amqpLoop
			}
			//log.Printf("[amqp receiver] Message body:\n\t%v", body)

			// Translate the body of the message into a P8Message object
			senderInfo := body["sender_info"].(map[interface{}]interface{})
			p8Message := P8Message {
				Encoding:   message.ContentEncoding,
				MsgTypeVal: MsgType(body["msgtype"].(int64)),
				MsgOpVal:   MsgOp(body["msgop"].(int64)),
				TimeStamp:  string(body["timestamp"].([]uint8)),
				SenderInfo: SenderInfo{
					Package:  string(senderInfo["package"].([]uint8)),
					Exe:      string(senderInfo["exe"].([]uint8)),
					Version:  string(senderInfo["version"].([]uint8)),
					Commit:   string(senderInfo["commit"].([]uint8)),
					Hostname: string(senderInfo["hostname"].([]uint8)),
					Username: string(senderInfo["username"].([]uint8)),
				},
				Payload: body["payload"],
			}
			routingKeyParts := strings.Split(message.RoutingKey, TargetSeparator)
			if len(routingKeyParts) > 1 {
				p8Message.Target = routingKeyParts[1:len(routingKeyParts)]
			}

			//log.Printf("[amqp receiver] Message:\n\t%v", p8Message)

			// Deal with the message according to the target
			if len(p8Message.Target) == 0 {
				log.Printf("[amqp receiver] No Hornet target provided")
			} else {
				switch p8Message.Target[0] {
				case "quit-hornet":
					reqQueue <- StopExecution
				default:
					log.Printf("[amqp receiver] Unknown hornet target: %v", p8Message.Target)
				}
			}
		}
	}

	log.Print("[amqp receiver] finished.")

}

// AmqpSender is a goroutine responsible for sending AMQP messages received on a channel
func AmqpSender(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	// decrement the wg counter at the end
	defer poolCount.Done()

	// Connect to the AMQP broker
	// Deferred command: close the connection
	brokerAddress := viper.GetString("amqp.broker")
	if strings.HasPrefix(brokerAddress, "amqp://") == false {
		brokerAddress = "amqp://" + brokerAddress
	}
	connection, receiveErr := amqp.Dial(brokerAddress)
	if receiveErr != nil {
		log.Printf("[amqp sender] Unable to connect to the AMQP broker at (%s) for receiving:\n\t%v", brokerAddress, receiveErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer connection.Close()

	// Create the channel object that represents the connection to the broker
	// Deferred command: close the channel
	channel, chanErr := connection.Channel()
	if chanErr != nil {
		log.Printf("[amqp sender] Unable to get the AMQP channel:\n\t%v", chanErr.Error())
		reqQueue <- StopExecution
		return
	}
	defer channel.Close()

	exchangeName := viper.GetString("amqp.exchange")

	log.Print("[amqp sender] started successfully")
	AmqpSenderIsActive = true
	defer func() {AmqpSenderIsActive = false}()

amqpLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-ctrlQueue:
			if controlMsg == StopExecution {
				log.Print("[amqp sender] stopping on interrupt.")
				break amqpLoop
			}
		// process any message reuqests receivec on the send-messsage queue
		case p8Message := <-SendMessageQueue:
			// Translate the request into a map that can be encoded for transmission
			var senderInfo = map[string]interface{} {
				"package": p8Message.SenderInfo.Package,
				"exe": p8Message.SenderInfo.Exe,
				"version": p8Message.SenderInfo.Version,
				"commit": p8Message.SenderInfo.Commit,
				"hostname": p8Message.SenderInfo.Hostname,
				"username": p8Message.SenderInfo.Username,
			}
			var body = map[string]interface{} {
				"msgtype": p8Message.MsgTypeVal,
				"msgop": p8Message.MsgOpVal,
				"timestamp": p8Message.TimeStamp,
				"sender_info": senderInfo,
				"payload": p8Message.Payload,
			}

			//log.Printf("[amqp sender] Received message to send:\n\t%v", body)
			bodyNBytes := unsafe.Sizeof(p8Message)
			//log.Printf("[amqp sender] Message size in bytes: %d", bodyNBytes)

			var message = amqp.Publishing {
				ContentEncoding: p8Message.Encoding,
				Body: make([]byte, 0, bodyNBytes),
			}
			// Encode the message body for transmission
			switch p8Message.Encoding {
			case "application/json":
				//log.Printf("this will be a json message")
				handle := new(codec.JsonHandle)
				encoder := codec.NewEncoderBytes(&(message.Body), handle)
				jsonErr := encoder.Encode(&body)
				if jsonErr != nil {
					log.Printf("[amqp sender] Unable to decode JSON-encoded message:\n\t%v", jsonErr)
					continue amqpLoop
				}
			case "application/msgpack":
				//log.Printf("this will be a msgpack message")
				handle := new(codec.MsgpackHandle)
				encoder := codec.NewEncoderBytes(&(message.Body), handle)
				msgpackErr := encoder.Encode(&body)
				if msgpackErr != nil {
					log.Printf("[amqp sender] Unable to decode msgpack-encoded message:\n\t%v", msgpackErr)
					continue amqpLoop
				}
			default:
				log.Printf("[amqp sender] Message content cannot be encoded with type <%s>", p8Message.Encoding)
				continue amqpLoop
			}

			routingKey := strings.Join(p8Message.Target, TargetSeparator)

			//log.Printf("[amqp sender] Encoded message:\n\t%v", message)

			// Publish!
			pubErr := channel.Publish(exchangeName, routingKey, false, false, message)
			if pubErr != nil {
				log.Printf("[amqp sender] Error while sending message:\n\t%v", pubErr)
			}

		}
	}

	log.Print("[amqp sender] finished.")
}

// PrepareMessage sets up most of the fields in a P8Message object.
// The payload is not set here.
func PrepareMessage(target []string, encoding string, msgType MsgType, msgOp MsgOp)(p8Message P8Message) {
	p8Message = P8Message {
		Target: target,
		Encoding: encoding,
		MsgTypeVal: msgType,
		MsgOpVal: msgOp,
		// timestamp
		SenderInfo: MasterSenderInfo,
	}
	return
}
