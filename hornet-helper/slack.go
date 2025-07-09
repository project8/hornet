/*
* slack.go
*
* Contains the contents of the slack library at github.com/Bowery/slack, at commit 12fa50d414
* Source obtained on June 12, 2015.
*
* Includes adaptations for use in Hornet.
*
 */

/*
* Original License:
*
The MIT License (MIT)

Copyright (c) 2015 Bowery

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package hornet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var (
	slackAddr = "https://slack.com/api"
)

const (
	postMessageURI = "chat.postMessage"
)

type slackPostMessageRes struct {
	Ok    bool
	Error string
}

// sendSlackMessage sends a text message to a specific channel
// with a specific username.
func sendSlackMessage(channel, message, username, token string) error {
	if channel == "" || message == "" || username == "" || token == "" {
		return errors.New("channel, message, username, and token are required")
	}
	host, _ := os.Hostname()
	message = "[" + host + "]" + message

	payload := url.Values{}
	payload.Set("token", token)
	payload.Set("channel", channel)
	payload.Set("text", message)
	payload.Set("username", username)
	payload.Set("as_user", "true")

	res, err := http.PostForm(fmt.Sprintf("%s/%s", slackAddr, postMessageURI), payload)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	resBody := new(slackPostMessageRes)
	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(resBody)
	if err != nil {
		return err
	}

	if !resBody.Ok {
		return errors.New(resBody.Error)
	}

	return nil
}

// Hornet-specific Slack usage

// configuration
var slackActive bool
var slackAlertsChannel, slackNoticesChannel string

//var slackUsername string
//var slackPrefix string

var slackFormat = logging.MustStringFormatter(
	"%{level} > %{message}",
)

// Notice writer
type SlackNoticeWriter struct {
	client *SlackClient
}

var slackNoticeWriter SlackNoticeWriter

func (writer SlackNoticeWriter) Write(p []byte) (n int, err error) {
	n = 0
	if slackActive == false {
		return
	}
	if err = writer.client.SubmitMessage(slackNoticesChannel, string(p)); err == nil {
		n = len(p)
	}
	return
}

// Alert writer
type SlackAlertWriter struct {
	client *SlackClient
}

var slackAlertWriter SlackAlertWriter

func (writer SlackAlertWriter) Write(p []byte) (n int, err error) {
	n = 0
	if slackActive == false {
		return
	}
	if err = writer.client.SubmitMessage(slackAlertsChannel, string(p)); err == nil {
		n = len(p)
	}
	return
}

// SlackClient represents a slack api client. A Client is
// used for making requests to the slack api.
type SlackClient struct {
	username       string
	token          string
	running        bool
	messageBuffers map[string]string
	messageMutex   sync.Mutex
}

// createNewSlackClient returns a Client with the provided api token.
func createNewSlackClient(username, token string) *SlackClient {
	var client = SlackClient{
		username:       username,
		token:          token,
		running:        false,
		messageBuffers: make(map[string]string),
	}
	return &client
}

// RunClient starts sending submitted messages via the Slack API
func (c *SlackClient) RunClient(ctrlQueue, reqQueue chan ControlMessage, poolCount *sync.WaitGroup) {
	if c.running == true {
		return
	}

	// decrement the wg counter at the end
	defer func(c *SlackClient) { c.running = false }(c)
	defer poolCount.Done()
	defer Log.Info("Slack client is finished.")

	const (
		maxErrors = 10
	)
	var errorCount int
	errorCount = 0

	timeout := make(chan bool, 1)
	timeout <- true
slackLoop:
	for {
		select {
		case controlMsg := <-ctrlQueue:
			if controlMsg == StopExecution {
				Log.Info("Slack client stopping on interrupt.")
				break slackLoop
			}
		case <-timeout:
			c.messageMutex.Lock()
			for channel, buffer := range c.messageBuffers {
				if buffer != "" {
					if msgErr := sendSlackMessage(channel, buffer, c.username, c.token); msgErr != nil {
						errorCount++
						if errorCount == maxErrors {
							Log.Error("Maximum number of Slack errors has been reached")
							break slackLoop
						}
					}
					c.messageBuffers[channel] = ""
				}
			}
			c.messageMutex.Unlock()
			go func() {
				time.Sleep(1 * time.Second)
				timeout <- true
			}()
		} // select
	} // for
} // func

// SubmitMessage submits a single message on a particular channel to the Slack client.
func (c *SlackClient) SubmitMessage(channel, message string) error {
	c.messageMutex.Lock()
	Log.Debugf("Submitting message (%v) <%v>", channel, message)
	if _, hasBuffer := c.messageBuffers[channel]; hasBuffer {
		c.messageBuffers[channel] += "\n" + message
	} else {
		c.messageBuffers[channel] = message
	}
	c.messageMutex.Unlock()
	return nil
}

// InitializeSlack creates the SlackClient object, gets the API token, and sends an initial
// message to the notice channel to ensure that the connection works.
func InitializeSlack(ctrlQueue, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) (e error) {
	if slackActive {
		return
	}

	// check whether slack activation has been requested in the config
	slackActive = viper.GetBool("slack.active")
	if slackActive == false {
		Log.Info("Slack not activated")
		return
	}

	// get the authentication token
	if Authenticators.Slack.Available == false {
		e = errors.New("Authentication not available for Slack")
		Log.Error(e.Error())
		return
	}

	// alert and notice channel names
	slackAlertsChannel = viper.GetString("slack.alerts-channel")
	slackNoticesChannel = viper.GetString("slack.notices-channel")

	Log.Debug("slack username: %v", viper.GetString("slack.username"))

	slackClient := createNewSlackClient(viper.GetString("slack.username"), Authenticators.Slack.Token)

	/*
		if SendSlackNotice("Hello Slack!") == false {
			e = errors.New("[slack] unable to send a message to slack")
			Log.Error(e.Error())
			return
		}
	*/

	// Notice logging
	slackNoticeWriter.client = slackClient
	slackNoticeBackend := logging.NewLogBackend(slackNoticeWriter, "", 0)
	slackNoticeBackendFormatter := logging.NewBackendFormatter(slackNoticeBackend, slackFormat)
	slackNoticeBackendLeveled := logging.AddModuleLevel(slackNoticeBackendFormatter)
	slackNoticeBackendLeveled.SetLevel(logging.NOTICE, "")
	AddBackend(slackNoticeBackendLeveled)
	//Log.Notice("Test Notice")

	// Alert logging
	slackAlertWriter.client = slackClient
	slackAlertBackend := logging.NewLogBackend(slackAlertWriter, "", 0)
	slackAlertBackendFormatter := logging.NewBackendFormatter(slackAlertBackend, slackFormat)
	slackAlertBackendLeveled := logging.AddModuleLevel(slackAlertBackendFormatter)
	slackAlertBackendLeveled.SetLevel(logging.ERROR, "")
	AddBackend(slackAlertBackendLeveled)
	//Log.Error("Test Alert")

	Log.Info("Starting Slack client")
	poolCount.Add(1)
	threadCountQueue <- 1
	go slackClient.RunClient(ctrlQueue, reqQueue, poolCount)

	Log.Info("Slack initialization complete")
	return
}

/*
// SendSlackAlert sends a message to the alert channel.
// Returns true if the message is sent, or if slack is not active.
func SendSlackAlert(message string) bool {
	if slackActive == false {return true}
	if slackErr := slackClient.sendSlackMessage(slackAlertsChannel, slackPrefix + message, slackUsername); slackErr != nil {
		Log.Error("Unable to send Slack alert: %v", slackErr.Error())
		return false
	}
	return true
}

// SendSlack Notice sends a message to the notice channel.
// Returns true if the message is sent, or if slack is not active.
func SendSlackNotice(message string) bool {
	if slackActive == false {return true}
	if slackErr := slackClient.sendSlackMessage(slackNoticesChannel, slackPrefix + message, slackUsername); slackErr != nil {
		Log.Error("Unable to send Slack notice: %v", slackErr.Error())
		return false
	}
	return true
}
*/
