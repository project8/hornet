/*
* authentication.go
*
* Loads and stores the authentication credentials that can be used by Hornet
*
* The current set of authenticators is:
*   - AMQP (username/password)
*   - Stack (token)
*/

package hornet

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os/user"
	"path/filepath"
)

type AmqpCredentialType struct {
	Username string `json:"user"`
	Password string `json:"password"`
	Available bool
}

type StackCredentialType struct {
	Token string `json:"token"`
	Available bool
}

type AuthenticatorsType struct {
	Amqp AmqpCredentialType `json:"amqp"`
	Stack StackCredentialType `json:"stack"`
}

var Authenticators AuthenticatorsType

func LoadAuthenticators() {
	// Get the home directory, where the authenticators live
    usr, err := user.Current()
    if err != nil {
        log.Fatal(err)
    }
    //log.Println( usr.HomeDir )

	// Read in the authenticators file
	authFilePath := filepath.Join(usr.HomeDir, ".project8_authentications.json")
	authFileData, fileErr := ioutil.ReadFile(authFilePath)
	if fileErr != nil {
		log.Fatal(fileErr)
	}

	// Unmarshal the JSON data
	if jsonErr := json.Unmarshal(authFileData, &Authenticators); jsonErr != nil {
		log.Fatal(jsonErr)
	}

	// Check which autheticators are actually available
	// AMQP
	if len(Authenticators.Amqp.Username) > 0 && len(Authenticators.Amqp.Password) > 0 {
		Authenticators.Amqp.Available = true
	}

	// Stack
	if len(Authenticators.Stack.Token) > 0 {
		Authenticators.Stack.Available = true
	}

	log.Printf("[authentication] authenticators ready for use:\n\t%s%t\n\t%s%t",
		"AMQP: ", Authenticators.Amqp.Available,
		"Stack: ", Authenticators.Stack.Available,
	)

	return
}


