/*
* shipper.go
*
* the shipper thread sends data files to a remote destination for long-term storage via rsync
 */

package hornet

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

func Shipper(context OperatorContext) {
	// decrement the wg counter at the end
	defer context.PoolCount.Done()
	defer log.Print("[shipper] finished.")

	remoteShip := false
	var destDirBase, hostname, username string
	if viper.IsSet("shipper.hostname") {
		remoteShip = true
		hostname = viper.GetString("shipper.hostname")
		username = viper.GetString("shipper.username")
		destDirBase = viper.GetString("shipper.dest-dir")
	} else {
		// for local ship, make the destination directory an absolute path
		destDirBase, _ = filepath.Abs(viper.GetString("shipper.dest-dir"))
	}

	log.Print("[shipper] started successfully")

shipLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				log.Print("[shipper] stopping on interrupt.")
				break shipLoop
			}
		case fileHeader := <-context.FileStream:
			opReturn := OperatorReturn{
				Operator: "shipper",
				FHeader:  fileHeader,
				Err:      nil,
				IsFatal:  false,
			}

			//inputFilePath := filepath.Join(fileHeader.WarmPath, fileHeader.Filename)
			inputFileSubPath := filepath.Clean(filepath.Join(fileHeader.SubPath, fileHeader.Filename))

			destDirPath := filepath.Clean(filepath.Join(destDirBase, fileHeader.SubPath))
			opReturn.FHeader.ColdPath = destDirPath

			var rsyncDest string
			if remoteShip {
				if len(username) > 0 {
					rsyncDest = username + "@" + hostname + ":" + destDirBase
				} else {
					rsyncDest = hostname + ":" + destDirBase
				}
			} else {
				rsyncDest = destDirBase
			}
			log.Printf("[shipper] rsync dest: %s", rsyncDest)

			//outputFilePath := filepath.Join(destDirPath, fileHeader.Filename)

			cmd := exec.Command("rsync", "-a", "--relative", inputFileSubPath, rsyncDest)
			// Set the command's working directory to the input basepath, 
			// so that the inputFileSubPath is definitely referring to the file.
			// The input basepath is the warm path minus the subpath
			inputBaseDir := strings.TrimSuffix(filepath.Clean(opReturn.FHeader.WarmPath), filepath.Clean(fileHeader.SubPath))
			cmd.Dir = filepath.Clean(inputBaseDir)
			log.Printf("[shipper] rsync command is: %v", cmd)

			// run the process
			outputError := cmd.Run()
			if outputError != nil {
				opReturn.Err = fmt.Errorf("Error on running rsync for <%s>: %v", fileHeader.Filename, outputError)
				log.Print("[shipper]", opReturn.Err.Error())
			}

			context.RetStream <- opReturn
		}

	}
}
