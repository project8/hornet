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

// SlackClient represents a slack api client. A Client is
// used for making requests to the slack api.
type SlackClient struct {
	token string
}

// createNewSlackClient returns a Client with the provided api token.
func createNewSlackClient(token string) *SlackClient {
	return &SlackClient{token}
}

// sendSlackMessage sends a text message to a specific channel
// with a specific username.
func (c *SlackClient) sendSlackMessage(channel, message, username string) error {
	if channel == "" || message == "" || username == "" {
		return errors.New("channel, message, and username required")
	}

	payload := url.Values{}
	payload.Set("token", c.token)
	payload.Set("channel", channel)
	payload.Set("text", message)
	payload.Set("username", username)

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
var slackUsername string
var slackPrefix string

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
	if slackActive == false {return}
	if err = writer.client.sendSlackMessage(slackNoticesChannel, string(p), slackUsername); err == nil {
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
	if slackActive == false {return}
	if err = writer.client.sendSlackMessage(slackAlertsChannel, string(p), slackUsername); err == nil {
		n = len(p)
	}
	return
}

// InitializeSlack creates the SlackClient object, gets the API token, and sends an initial 
// message to the notice channel to ensure that the connection works.
func InitializeSlack() (e error) {
	if slackActive {return}

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

	// prefix for messages sent to slack (identifies their origin)
	if viper.IsSet("slack.prefix") {
		slackPrefix = viper.GetString("slack.prefix")
	} else {
		slackPrefix = "[hornet] "
	}

	// username
	slackUsername = viper.GetString("slack.username")

	// alert and notice channel names
	slackAlertsChannel = viper.GetString("slack.alerts-channel")
	slackNoticesChannel = viper.GetString("slack.notices-channel")

	slackClient := createNewSlackClient(Authenticators.Slack.Token)

	/*
	if SendSlackNotice("Hello Slack!") == false {
		e = errors.New("[slack] unable to send a message to slack")
		Log.Error(e.Error())
		return
	}
	*/

	// Notice logging
	slackNoticeWriter.client = slackClient
	slackNoticeBackend := logging.NewLogBackend(slackNoticeWriter, slackPrefix, 0)
	slackNoticeBackendFormatter := logging.NewBackendFormatter(slackNoticeBackend, slackFormat)
	slackNoticeBackendLeveled := logging.AddModuleLevel(slackNoticeBackendFormatter)
	slackNoticeBackendLeveled.SetLevel(logging.NOTICE, "")
	AddBackend(slackNoticeBackendLeveled)
	Log.Notice("Test Notice")

	// Alert logging
	slackAlertWriter.client = slackClient
	slackAlertBackend := logging.NewLogBackend(slackAlertWriter, slackPrefix, 0)
	slackAlertBackendFormatter := logging.NewBackendFormatter(slackAlertBackend, slackFormat)
	slackAlertBackendLeveled := logging.AddModuleLevel(slackAlertBackendFormatter)
	slackAlertBackendLeveled.SetLevel(logging.ERROR, "")
	AddBackend(slackAlertBackendLeveled)
	Log.Error("Test Alert")

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

