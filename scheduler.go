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
        "sync"
)


type OperatorReturn struct {
        Operator string
        InFile   string
        OutFile  string
        Err      error
}

type OperatorContext struct {
        FileStream chan string
        RetStream  chan OperatorReturn
        CtrlQueue  chan ControlMessage
        ReqQueue   chan ControlMessage
        PoolCount  *sync.WaitGroup
}


func Scheduler(schQueue chan string, ctrlQueue chan ControlMessage, reqQueue chan ControlMessage, poolSize uint, poolCount *sync.WaitGroup, config Config) {
	// Decrement the waitgroup counter when done
	defer poolCount.Done()

        // create the file queues
        moverQueue    := make(chan string, 3 * poolSize)
        workerQueue   := make(chan string, 3 * poolSize)
        //shipperQueue  := make(chan string, 3 * poolSize)
        
        // create the return queues
        moverRetQueue   := make(chan OperatorReturn, 3 * poolSize)
        workerRetQueue  := make(chan OperatorReturn, 3 * poolSize)
        shipperRetQueue := make(chan OperatorReturn, 3 * poolSize)

        // setup the mover
        moverCtx := OperatorContext{
                FileStream: moverQueue,
                RetStream:  moverRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
	go Mover(moverCtx, config)

        // setup the workers
        workerCtx := OperatorContext{
                FileStream: workerQueue,
                RetStream:  workerRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	// build the work pool.  This is config.PoolSize worker threads
	for i := uint(0); i < poolSize; i++ {
		poolCount.Add(1)
		go Worker(workerCtx, config, WorkerID(i))
	}
/*
        // setup the shipper
        shipperCtx := OperatorContext{
                FileStream: shipperQueue,
                RetStream:  shipperRetQueue,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
	go Shipper(shipperCtx, config)
*/
        // setup the watcher
        watcherCtx := OperatorContext{
                FileStream: schQueue,
                RetStream:  nil,
                CtrlQueue:  ctrlQueue,
                ReqQueue:   reqQueue,
                PoolCount:  poolCount,
        }
	poolCount.Add(1)
	go Watcher(watcherCtx, config)

        var workersWorking uint
        workersWorking = 0


scheduleLoop:
        for {
                select {
                case controlMsg := <-ctrlQueue:
                        if controlMsg == StopExecution {
                                log.Print("[scheduler] scheduler stopping on interrupt")
                                // close the worker queue to stop the workers
                                close(workerQueue)
                                break scheduleLoop
                        }
                case file := <-schQueue:
                        log.Printf("[scheduler] sending <%s> to the mover", file)
                        moverQueue <- file
                case fileRet := <-moverRetQueue:
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the mover: %v", fileRet.Err)
                        } else {
                                file := fileRet.OutFile // file has been moved, so we want the output file
                                // only send to the workers if there's a worker available
                                if workersWorking < poolSize {
                                        log.Printf("[scheduler] sending <%s> to the workers", file)
                                        workersWorking++
                                        workerQueue <- file
                                        fmt.Println("workers working:", workersWorking)
                                } else {
                                        log.Printf("[scheduler] sending <%s> to shipper (skipping workers)", file)
                                        // NO SHIPPER EXISTS YET, SO SKIP THE SHIPPER
                                        //shipperQueue <- file
                                        shipperRetQueue <- fileRet
                                }
                        }
                case fileRet := <-workerRetQueue:
                        workersWorking--
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the workers: %v", fileRet.Err)
                        } else {
                                file := fileRet.InFile // original data file is still the input file from the worker
                                log.Printf("[scheduler] sending <%s> to the shipper", file)
                                // NO SHIPPER EXISTS YET, SO SKIP THE SHIPPER
                                //shipperQueue <- file
                                shipperRetQueue <- fileRet
                        }
                case fileRet := <-shipperRetQueue:
                        if fileRet.Err != nil {
                                log.Printf("[scheduler] error received from the shipper: %v", fileRet.Err)
                        } else {
                                log.Printf("[scheduler] completed work on file <%s>", fileRet.InFile)
                        }
                }
        }


}

