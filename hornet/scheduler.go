/*
* scheduler.go
*
* The scheduler is responsible for scheduling files for processing and
* maintaining the channels for moving files from one component of
* hornet to the next.
 */

package hornet

import (
	"path/filepath"
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

func Scheduler(schQueue chan string, ctrlQueue, reqQueue chan ControlMessage, threadCountQueue chan uint, poolCount *sync.WaitGroup) {
	// Decrement the waitgroup counter when done
	defer poolCount.Done()
	defer Log.Info("Scheduler is finished.")

	queueSize := viper.GetInt("scheduler.queue-size")
	Log.Debug("Queue size: %d", queueSize)
	if queueSize <= 0 {
		Log.Critical("Queue size must be > 0")
		reqQueue <- ThreadCannotContinue
		return
	}

	nWorkers := viper.GetInt("workers.n-workers")
	Log.Debug("Number of workers: %d", nWorkers)
	if nWorkers <= 0 {
		Log.Critical("Number of workers must be > 0")
		reqQueue <- ThreadCannotContinue
		return
	}

	shipperIsActive := viper.GetBool("shipper.active")
	Log.Debug("Shipper active: %v", shipperIsActive)

	// for now, we require that there's only 1 shipper
	if viper.GetInt("shipper.n-shippers") != 1 {
		Log.Critical("Currently can only have 1 shipper")
		reqQueue <- ThreadCannotContinue
		return
	}

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

	if shipperIsActive {
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
	}

	// setup the watcher
	if viper.GetBool("watcher.active") {
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
	}

	workersWorking := int(0)

scheduleLoop:
	for {
		select {
		case controlMsg, queueOk := <-ctrlQueue:
			if ! queueOk {
				Log.Error("Control queue has closed unexpectedly")
				break scheduleLoop
			}
			if controlMsg == StopExecution {
				Log.Info("Scheduler stopping on interrupt")
				// close the worker queue to stop the workers
				close(workerQueue)
				break scheduleLoop
			}
		case file, queueOk := <-schQueue:
			if ! queueOk {
				Log.Error("Scheduler queue has closed unexpectedly")
				reqQueue <- StopExecution
				break scheduleLoop
			}
			if absPath, absErr := filepath.Abs(file); absErr != nil {
				Log.Error("Unable to determine an absolute path for <%s>", file)
			} else {
				if PathIsRegularFile(absPath) {
					path, filename := filepath.Split(absPath)
					fileHeader := FileInfo{
						Filename:     filename,
						HotPath:      path,
						FileHotPath:  absPath,
					}
					Log.Notice("Sending <%s> to the classifier", fileHeader.Filename)
					classifierQueue <- fileHeader
				} else {
					Log.Info("<%s> is not a regular file; ignoring", absPath)
				}
			}
		case fileRet, queueOk := <-classifierRetQueue:
			if ! queueOk {
				Log.Error("Classifier return queue has closed unexpectedly")
				reqQueue <- StopExecution
				break scheduleLoop
			}
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				Log.Info("Received %s from the classifier:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				fileHeader := fileRet.FHeader
				Log.Info("Sending <%s> to the mover", fileHeader.Filename)
				moverQueue <- fileHeader
			}
		case fileRet, queueOk := <-moverRetQueue:
			if ! queueOk {
				Log.Error("Mover return queue has closed unexpectedly")
				reqQueue <- StopExecution
				break scheduleLoop
			}
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				Log.Info("Received %s from the mover:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				fileHeader := fileRet.FHeader
				// only send to the workers if the file requests it and there's a worker available
				if len(fileHeader.JobQueue) > 0 && workersWorking < nWorkers {
					Log.Info("Sending <%s> to the workers", fileHeader.Filename)
					workersWorking++
					workerQueue <- fileHeader
					//fmt.Println("[scheduler] workers working:", workersWorking)
				} else {
					if shipperIsActive == true {
						Log.Info("Sending <%s> to shipper (skipping nearline)", fileHeader.Filename)
						shipperQueue <- fileHeader
					} else {
						Log.Notice("Completed work on file <%s>", fileRet.FHeader.Filename)
					}
				}
			}
		case fileRet, queueOk := <-workerRetQueue:
			if ! queueOk {
				Log.Error("Worker return queue has closed unexpectedly")
				reqQueue <- StopExecution
				break scheduleLoop
			}
			workersWorking--
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				Log.Info("Received %s from the workers:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				if shipperIsActive == true {
					fileHeader := fileRet.FHeader // original data file is still the input file from the worker
					Log.Info("Sending <%s> to the shipper", fileHeader.Filename)
					shipperQueue <- fileHeader
				} else {
					Log.Notice("Completed work on file <%s>", fileRet.FHeader.Filename)
				}
			}
		case fileRet, queueOk := <-shipperRetQueue:
			if ! queueOk {
				Log.Error("Shipper return queue has closed unexpectedly")
				reqQueue <- StopExecution
				break scheduleLoop
			}
			if fileRet.Err != nil {
				severity := "warning"
				if fileRet.IsFatal {
					severity = "error"
				}
				Log.Info("Received %s from the shipper:\n\t%v", severity, fileRet.Err)
			}
			if fileRet.IsFatal == false {
				Log.Notice("Completed work on file <%s>", fileRet.FHeader.Filename)
			}
		}
	}
}
