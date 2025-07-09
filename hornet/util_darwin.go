// Mac (Darwin)-only Utility functions for the hornet package.
package hornet

import (
	//"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Hash performs an md5 hash of the specified file and returns the hash string
// md5 is used on Mac systems.
// The output is expected to look like: MD5 (my_file) = f104f1956b7652b8b9c7b16398153a74
func Hash(filename string) (hash string, e error) {
	if hashOutput, hashErr := exec.Command("md5", filename).CombinedOutput(); hashErr != nil {
		e = fmt.Errorf("Error while hashing: %v", hashErr.Error())
	} else {
		hashTokens := strings.Fields(string(hashOutput))
		hash = hashTokens[3]
	}
	return
}
