/*
* shipper.go
*
* the shipper thread sends data files to a remote destination for long-term storage via rsync
*/

package main

import(
        "fmt"
	"log"
	"os/exec"
	"path/filepath"

        "github.com/spf13/viper"
)


func Shipper(context OperatorContext) {
	// decrement the wg counter at the end
	defer context.PoolCount.Done()

        destDir := viper.GetString("shipper.dest-dir")

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
		case inputFilePath := <-context.FileStream:
                        opReturn := OperatorReturn{
                                     Operator:  "shipper",
                                     InFile:    inputFilePath,
                                     OutFile:   "",
                                     Err:       nil,
                        }

                        _, inputFile := filepath.Split(inputFilePath)


                        outputFilePath := filepath.Join(destDir, inputFile)
                        opReturn.OutFile = outputFilePath
		        cmd := exec.Command("rsync", "-a", inputFilePath, outputFilePath)

		        // run the process
                        outputError := cmd.Run()
                        if outputError != nil {
                                opReturn.Err = fmt.Errorf("Error on running rsync for <%s>: %v", inputFilePath, outputError)
                                log.Print("[shipper]", opReturn.Err.Error())
                        }

                        context.RetStream <- opReturn
                }

	}

	// Finish any pending move jobs.

	log.Print("[shipper] finished.")
}
