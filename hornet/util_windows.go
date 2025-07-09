// Windows-only Utility functions for the hornet package.
package hornet

import (
	//"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Hash performs an md5 hash of the specified file and returns the hash string
// Microsoft File Checksum Integrity Verifier is used on Windows systems
// The output is expected to look like:
// //
// // File Checksum Integrity Verifier version 2.05.
// //
// f104f1956b7652b869c7b16398153a74 my_file
func Hash(filename string) (hash string, e error) {
	if hashOutput, hashErr := exec.Command("fciv", filename).CombinedOutput(); hashErr != nil {
		e = fmt.Errorf("Error while hashing: %v", hashErr.Error())
	} else {
		hashTokens := strings.Fields(string(hashOutput))
		hash = hashTokens[9]
	}
	return
}
