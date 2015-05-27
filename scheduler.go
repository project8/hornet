/*
* scheduler.go
*
* The scheduler is responsible for scheduling files for processing and 
* maintaining the channels for moving files from one component of 
* hornet to the next.
*/

package main

import (
        "fmt"
        "log"
        "path/filepath"
        "strings"
        "sync"

        "github.com/spf13/viper"
)


// File information header
type FileInfo struct {
        Filename        string
        FileType        string
        FileHash        string
        HotPath         string
        WarmPath        string
        ColdPath        string
        DoNearline      bool
        NearlineCmdName string
        NearlineCmdArgs []string
}

// SetNearlineCmd splits the command name from its arguments, and sets the name in the FileInfo struct
func (fileInfoPtr *FileInfo) SetNearlineCmd(nearlineCmd string) {
        fileInfo := *fileInfoPtr
        commandParts := strings.Fields(nearlineCmd)
        fileInfo.NearlineCmdName = commandParts[0]
        fileInfo.NearlineCmdArgs = commandParts[1:len(commandParts)]
        *fileInfoPtr = fileInfo
        return
}

type OperatorReturn struct {
        Operator string
        FHeader  FileInfo
        InFile   string
        OutFile  string
        Err      error
}

type OperatorContext struct {
        SchStream  chan string
        FileStream chan FileInfo
        RetStream  chan OperatorReturn
        CtrlQueue  chan ControlMessage
        ReqQueue   chan ControlMessage
        PoolCount  *sync.WaitGroup
}


func Scheduler(schQueue chan string, ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) {
	// Decrement the waitgroup counter when done
	defer poolCount.Done()

        queueSize := viper.GetInt("scheduler.queue-size")
        log.Println("[scheduler] Queue size:", queueSize)

        nWorkers := viper.GetInt("scheduler.n-nearline-workers")
        log.Println("[scheduler] Number of workers:", nWorkers)

        // create the file queues
        classifierQueue := make(chan FileInfo, queueSize)
        moverQueue      := make(chan FileInfo, queueSize)
        workerQueue     := make(chan FileInfo, queueSize)
        shipperQueue    := make(chan FileInfo, queueSize)
        
        // create the return queues
        classifierRetQueue := make(chan OperatorReturn, queueSize)
        moverRetQueue      := make(chan OperatorReturn, queueSize)
        workerRetQueue     := make(chan OperatorReturn, queueSize)
        shipperRetQueue    := make(chan OperatorReturn, queueSize)

        // setup the classifier
        classifierCtx := OperatorContext{
                SchStream:  schQueue,
                FileStream: classifierQueue,
                RetStream:  classifierRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
        threadCountQueue <- 1
	go Classifier(classifierCtx)

        // setup the mover
        moverCtx := OperatorContext{
                SchStream:  schQueue,
                FileStream: moverQueue,
                RetStream:  moverRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
        threadCountQueue <- 1
	go Mover(moverCtx)

        // setup the workers
        workerCtx := OperatorContext{
                SchStream:  schQueue,
                FileStream: workerQueue,
                RetStream:  workerRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	for i := int(0); i < nWorkers; i++ {
		poolCount.Add(1)
                threadCountQueue <- 1
		go Worker(workerCtx, WorkerID(i))
	}

        // setup the shipper
        shipperCtx := OperatorContext{
                SchStream:  schQueue,
                FileStream: shipperQueue,
                RetStream:  shipperRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
        threadCountQueue <- 1
	go Shipper(shipperCtx)

        // setup the watcher
        watcherCtx := OperatorContext{
                SchStream:  schQueue,
                FileStream: nil,
                RetStream:  nil,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
        threadCountQueue <- 1
	go Watcher(watcherCtx)

        workersWorking := int(0)


scheduleLoop:
        for {
                select {
                case controlMsg := <-ctrlQueue:
                        if controlMsg == StopExecution {
                                log.Print("[scheduler] stopping on interrupt")
                                // close the worker queue to stop the workers
                                close(workerQueue)
                                break scheduleLoop
                        }
                case file := <-schQueue:
                        path, filename := filepath.Split(file)
                        fileHeader := FileInfo{
                                Filename: filename,
                                FileType: "",
                                HotPath: path,
                                WarmPath: "",
                                ColdPath: "",
                                DoNearline: false,
                                NearlineCmdName: "",
                                NearlineCmdArgs: []string{},
                        }
                        log.Printf("[scheduler] sending <%s> to the classifier", fileHeader.Filename)
                        classifierQueue <- fileHeader
                case fileRet := <-classifierRetQueue:
                        if fileRet.Err != nil {
                                log.Printf("[scheduler error received from the classifier: %v", fileRet.Err)
                        } else {
                                fileHeader := fileRet.FHeader
                                log.Printf("[scheduler] sending <%s> to the mover", fileHeader.Filename)
                                moverQueue <- fileHeader
                        }
                case fileRet := <-moverRetQueue:
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the mover: %v", fileRet.Err)
                        } else {
                                fileHeader := fileRet.FHeader
                                // only send to the nearline workers if the file requests it and there's a worker available
                                if fileHeader.DoNearline && workersWorking < nWorkers {
                                        log.Printf("[scheduler] sending <%s> to the workers", fileHeader.Filename)
                                        workersWorking++
                                        workerQueue <- fileHeader
                                        fmt.Println("workers working:", workersWorking)
                                } else {
                                        log.Printf("[scheduler] sending <%s> to shipper (skipping nearline)", fileHeader.Filename)
                                        shipperQueue <- fileHeader
                                }
                        }
                case fileRet := <-workerRetQueue:
                        workersWorking--
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the workers: %v", fileRet.Err)
                        } else {
                                fileHeader := fileRet.FHeader // original data file is still the input file from the worker
                                log.Printf("[scheduler] sending <%s> to the shipper", fileHeader.Filename)
                                shipperQueue <- fileHeader
                        }
                case fileRet := <-shipperRetQueue:
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the shipper: %v", fileRet.Err)
                        } else {
                                log.Printf("[scheduler] completed work on file <%s>", fileRet.FHeader.Filename)
                        }
                }
        }


}

