/*
* authentication.go
*
* Loads and stores the authentication credentials that can be used by Hornet
*
* The current set of authenticators is:
*   - AMQP (username/password)
*   - Slack (token)
*/

package hornet

import (
	"encoding/json"
	"io/ioutil"
	"os/user"
	"path/filepath"
)

type AmqpCredentialType struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Available bool
}

type SlackCredentialType struct {
	Token string `json:"hornet"`
	Available bool
}

type AuthenticatorsType struct {
	Amqp AmqpCredentialType `json:"amqp"`
	Slack SlackCredentialType `json:"slack"`
}

var Authenticators AuthenticatorsType

func LoadAuthenticators() (e error) {
	// Get the home directory, where the authenticators live
    usr, usrErr := user.Current()
    if usrErr != nil {
        e = usrErr
		Log.Error(e.Error())
    }
    //log.Println( usr.HomeDir )

	// Read in the authenticators file
	authFilePath := filepath.Join(usr.HomeDir, ".project8_authentications.json")
	authFileData, fileErr := ioutil.ReadFile(authFilePath)
	if fileErr != nil {
		e = fileErr
		Log.Error(e.Error())
	}

	// Unmarshal the JSON data
	if jsonErr := json.Unmarshal(authFileData, &Authenticators); jsonErr != nil {
		e = jsonErr
		Log.Error(e.Error())
	}

	// Check which autheticators are actually available
	// AMQP
	if len(Authenticators.Amqp.Username) > 0 && len(Authenticators.Amqp.Password) > 0 {
		Authenticators.Amqp.Available = true
	}

	// Slack
	if len(Authenticators.Slack.Token) > 0 {
		Authenticators.Slack.Available = true
	}

	Log.Info("Authenticators ready for use:\n\t%s%t\n\t%s%t",
		"AMQP: ", Authenticators.Amqp.Available,
		"Slack: ", Authenticators.Slack.Available,
	)

	return
}


