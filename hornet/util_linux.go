// Linux-only Utility functions for the hornet package.
package hornet

import (
	//"errors"
	"fmt"
	"os/exec"
	"strings"
)


// Hash performs an md5 hash of the specified file and returns the hash string
// md5sum is used on Linux systems
// The output is expected to look like: dc5e29010b13215bbf8cfc6997f15489  my_file
func Hash(filename string) (hash string, e error) {
	if hashOutput, hashErr := exec.Command("md5sum", "-b", filename).CombinedOutput(); hashErr != nil {
		e = fmt.Errorf("Error while hashing: %v", hashErr.Error())
	} else {
		hashTokens := strings.Fields(string(hashOutput))
		hash = hashTokens[0]
	}
	return
}