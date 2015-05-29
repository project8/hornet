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
	//"strings"
	"sync"

	"github.com/spf13/viper"
)

type OperatorReturn struct {
	Operator string
	FHeader  FileInfo
	Err      error
	IsFatal  bool
}

type OperatorContext struct {
	SchStream        chan string
	FileStream       chan FileInfo
	RetStream        chan OperatorReturn
	CtrlQueue        chan ControlMessage
	ReqQueue         chan ControlMessage
	ThreadCountQueue chan uint
	PoolCount        *sync.WaitGroup
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
	moverQueue := make(chan FileInfo, queueSize)
	workerQueue := make(chan FileInfo, queueSize)
	shipperQueue := make(chan FileInfo, queueSize)

	// create the return queues
	classifierRetQueue := make(chan OperatorReturn, queueSize)
	moverRetQueue := make(chan OperatorReturn, queueSize)
	workerRetQueue := make(chan OperatorReturn, queueSize)
	shipperRetQueue := make(chan OperatorReturn, queueSize)

	// setup the classifier
	classifierCtx := OperatorContext{
		SchStream:        schQueue,
		FileStream:       classifierQueue,
		RetStream:        classifierRetQueue,
		CtrlQueue:        ctrlQueue,
		ReqQueue:         reqQueue,
		ThreadCountQueue: threadCountQueue,
		PoolCount:        poolCount,
	}
	poolCount.Add(1)
	threadCountQueue <- 1
	go Classifier(classifierCtx)

	// setup the mover
	moverCtx := OperatorContext{
		SchStream:        schQueue,
		FileStream:       moverQueue,
		RetStream:        moverRetQueue,
		CtrlQueue:        ctrlQueue,
		ReqQueue:         reqQueue,
		ThreadCountQueue: threadCountQueue,
		PoolCount:        poolCount,
	}
	poolCount.Add(1)
	threadCountQueue <- 1
	go Mover(moverCtx)

	// setup the workers
	workerCtx := OperatorContext{
		SchStream:        schQueue,
		FileStream:       workerQueue,
		RetStream:        workerRetQueue,
		CtrlQueue:        ctrlQueue,
		ReqQueue:         reqQueue,
		ThreadCountQueue: threadCountQueue,
		PoolCount:        poolCount,
	}
	for i := int(0); i < nWorkers; i++ {
		poolCount.Add(1)
		threadCountQueue <- 1
		go Worker(workerCtx, WorkerID(i))
	}

	// setup the shipper
	shipperCtx := OperatorContext{
		SchStream:        schQueue,
		FileStream:       shipperQueue,
		RetStream:        shipperRetQueue,
		CtrlQueue:        ctrlQueue,
		ReqQueue:         reqQueue,
		ThreadCountQueue: threadCountQueue,
		PoolCount:        poolCount,
	}
	poolCount.Add(1)
	threadCountQueue <- 1
	go Shipper(shipperCtx)

	// setup the watcher
	watcherCtx := OperatorContext{
		SchStream:        schQueue,
		FileStream:       nil,
		RetStream:        nil,
		CtrlQueue:        ctrlQueue,
		ReqQueue:         reqQueue,
		ThreadCountQueue: threadCountQueue,
		PoolCount:        poolCount,
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
			if absPath, absErr := filepath.Abs(file); absErr != nil {
				log.Printf("[scheduler] unable to determine an absolute path for <%s>", file)
			} else {
				path, filename := filepath.Split(absPath)
				fileHeader := FileInfo{
					Filename:   filename,
					HotPath:    path,
					DoNearline: false,
				}
				log.Printf("[scheduler] sending <%s> to the classifier", fileHeader.Filename)
				classifierQueue <- fileHeader
			}
		case fileRet := <-classifierRetQueue:
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				log.Printf("[scheduler] %s received from the classifier:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				fileHeader := fileRet.FHeader
				log.Printf("[scheduler] sending <%s> to the mover", fileHeader.Filename)
				moverQueue <- fileHeader
			}
		case fileRet := <-moverRetQueue:
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				log.Printf("[scheduler] %s received from the mover:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
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
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				log.Printf("[scheduler] %s received from the workers:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				fileHeader := fileRet.FHeader // original data file is still the input file from the worker
				log.Printf("[scheduler] sending <%s> to the shipper", fileHeader.Filename)
				shipperQueue <- fileHeader
			}
		case fileRet := <-shipperRetQueue:
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				log.Printf("[scheduler] %s received from the shipper:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				log.Printf("[scheduler] completed work on file <%s>", fileRet.FHeader.Filename)
			}
		}
	}

}
