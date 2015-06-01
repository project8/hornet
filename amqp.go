/*
* amqp.go
*
* the amqp functions takes care of communication via the AMQP protocol
*
* Two threads can be used for receiving and sending AMQP messages, respectively: AmqpReceiver and AmqpSender
 */

package main

import (
	"errors"
	"log"
	//"os/exec"
	//"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"github.com/ugorji/go/codec"
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
	MsgType    uint64
	MsgOp      uint64
	TimeStamp  string
	SenderInfo
	Payload    interface{}
}

// ValidateAmqpConfig checks the sanity of the amqp section of a configuration.
// It makes the following guarantees
//   1) The broker setting is present
//   2) If the receiver is present and active, then the queue and exchange are set.
func ValidateAmqpConfig() (e error) {
	if viper.IsSet("amqp.broker") == false {
		// if the broker isn't there, we won't use the amqp receiver or sender
		return
	}

	if viper.IsSet("amqp.recever") && viper.GetBool("amqp.receiver.active") {
		if viper.IsSet("amqp.receiver.queue") == false || viper.IsSet("amqp.receiver.exchange") == false {
			e = errors.New("[amqp] Receiver section is missing the queue or exchange")
			log.Print(e.Error())
		}
	}

	if viper.IsSet("amqp.sender") && viper.GetBool("amqp.sender.active") {
	}

	return
}

func StartAmqp(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) {
	log.Print("[amqp] Starting AMQP components")
	if viper.IsSet("amqp.broker") == false {
		log.Print("[amqp] No AMQP broker specified")
		return
	}
	if viper.IsSet("amqp.receiver") && viper.GetBool("amqp.receiver.active") {
		log.Print("[amqp] Starting AMQP receiver")
		poolCount.Add(1)
		threadCountQueue <- 1
		go AmqpReceiver(ctrlQueue, reqQueue, poolCount)
	}
	/*
	   if viper.IsSet("amqp.sender") && viper.GetBool("amqp.sender.active") {
	           log.Print("[amqp] Starting AMQP sender")
	           poolCount.Add(1)
	           threadCountQueue <- 1
	           go AmqpSender(ctrlQueue, reqQueue, poolCount)
	   }
	*/
	return
}

func AmqpReceiver(ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	// decrement the wg counter at the end
	defer poolCount.Done()

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

	channel, chanErr := connection.Channel()
	if chanErr != nil {
		log.Printf("[amqp receiver] Unable to get the AMQP channel:\n\t%v", chanErr.Error())
		reqQueue <- StopExecution
		return
	}

	exchangeName := viper.GetString("amqp.receiver.exchange")
	exchDeclErr := channel.ExchangeDeclare(exchangeName, "topic", false, false, false, false, nil)
	if exchDeclErr != nil {
		log.Printf("[amqp receiver] Unable to declare exchange <%s>:\n\t%v", exchangeName, exchDeclErr.Error())
		reqQueue <- StopExecution
		return
	}

	queueName := viper.GetString("amqp.receiver.queue")
	_, queueDeclErr := channel.QueueDeclare(queueName, false, true, true, false, nil)
	if queueDeclErr != nil {
		log.Printf("[amqp receiver] Unable to declare queue <%s>:\n\t%v", queueName, queueDeclErr.Error())
		reqQueue <- StopExecution
		return
	}

	queueBindErr := channel.QueueBind(queueName, queueName+".#", exchangeName, false, nil)
	if queueBindErr != nil {
		log.Printf("[amqp receiver] Unable to bind queue <%s> to exchange <%s>:\n\t%v", queueName, exchangeName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}

	messageQueue, consumeErr := channel.Consume(queueName, "", false, true, true, false, nil)
	if consumeErr != nil {
		log.Printf("[amqp receiver] Unable start consuming from queue <%s>:\n\t%v", queueName, queueBindErr.Error())
		reqQueue <- StopExecution
		return
	}

	log.Print("[amqp receiver] started successfully")

amqpLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-ctrlQueue:
			if controlMsg == StopExecution {
				log.Print("[amqp receiver] stopping on interrupt.")
				break amqpLoop
			}
		case message := <-messageQueue:
			message.Ack(false)
			log.Printf("[amqp receiver] Received message with encoding %s", message.ContentEncoding)
			var body map[string]interface{}
			switch message.ContentEncoding {
			case "application/json":
				log.Printf("this is a json message")
				handle := new(codec.JsonHandle)
				decoder := codec.NewDecoderBytes(message.Body, handle)
				jsonErr := decoder.Decode(&body)
				if jsonErr != nil {
					log.Printf("[amqp receiver] Unable to decode JSON-encoded message:\n\t%v", jsonErr)
					continue amqpLoop
				}
			case "application/msgpack":
				log.Printf("this is a msgpack message")
				handle := new(codec.MsgpackHandle)
				decoder := codec.NewDecoderBytes(message.Body, handle)
				msgpackErr := decoder.Decode(&body)
				if msgpackErr != nil {
					log.Printf("[amqp receiver] Unable to decode msgpack-encoded message:\n\t%v", msgpackErr)
					continue amqpLoop
				}
			default:
				log.Printf("[amqp receiver] Message content encoding is not understood: %s", message.ContentEncoding)
			}
			log.Printf("[amqp receiver] Message body:\n\t%v", body)

			senderInfo := body["sender_info"].(map[interface{}]interface{})
			p8Message := P8Message {
				MsgType: body["msgtype"].(uint64),
				MsgOp:   body["msgop"].(uint64),
				TimeStamp: body["timestamp"].(string),
				SenderInfo: SenderInfo{
					Package:  senderInfo["package"].(string),
					Exe:      senderInfo["exe"].(string),
					Version:  senderInfo["version"].(string),
					Commit:   senderInfo["commit"].(string),
					//Hostname: senderInfo["hostname"].(string),
					//Username: senderInfo["username"].(string),
				},
				Payload: body["payload"],
			}
			routingKeyParts := strings.Split(message.RoutingKey, ".")
			if len(routingKeyParts) > 1 {
				p8Message.Target = routingKeyParts[1:len(routingKeyParts)]
			}

			log.Printf("[amqp receiver] Message:\n\t%v", p8Message)

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

	log.Print("[amqp hub] finished.")

}
