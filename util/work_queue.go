package util

import (
	"context"
	"runtime"
	"sync"
	"time"
)

//MaxQueue 队列最大缓存数
const MaxQueue = 1024

//Job 任务
type Job interface {
	WorkFunc()
}

//Worker 工作者
type Worker struct {
	dispatcher *Dispatcher
	jobChan    chan Job
}

//NewWorker 新建
func NewWorker(d *Dispatcher) *Worker {
	return &Worker{
		dispatcher: d,
		jobChan:    make(chan Job),
	}
}

//run 运行
func (w *Worker) run() {
	for {
		j := <-w.jobChan
		if j == nil {
			break
		}
		j.WorkFunc()
		w.dispatcher.WorkerQueue <- w.jobChan
	}
	close(w.jobChan)
}

//Dispatcher 调度者
type Dispatcher struct {
	Ctx  context.Context
	Name string

	JobQueue    chan Job      //任务队列
	WorkerQueue chan chan Job //工作者队列

	currentWorkersCount     int
	dispatcherCheckDuration time.Duration //定时释放闲置的协程
	workerPool              []chan Job
	maxworkerPoolCount      int           //池最大的协程数
	stopChan                chan struct{} //退出信号
	stopFlag                int32         //退出标志
	closeOnce               sync.Once
	logger                  *Logger
	WaitGroupWrapper
}

//NewDispatcher 新建
func NewDispatcher(ctx context.Context, name string, maxWorkers int) *Dispatcher {
	logger, _ := NewLogger(DebugLevel, "")
	logger.SetMark("Dispatcher." + name)
	d := &Dispatcher{
		Ctx:  ctx,
		Name: name,

		JobQueue:    make(chan Job, MaxQueue),
		WorkerQueue: make(chan chan Job, MaxQueue),

		currentWorkersCount:     0,
		dispatcherCheckDuration: 2 * time.Second,
		workerPool:              make([]chan Job, maxWorkers),
		maxworkerPoolCount:      maxWorkers,
		stopChan:                make(chan struct{}),
		logger:                  logger,
	}
	for i := 0; i < d.maxworkerPoolCount; i++ {
		worker := NewWorker(d)
		d.workerPool[i] = worker.jobChan
		d.Wrap(worker.run)
	}
	return d
}

//PutJob 入队操作
//多路复用，运行任务的顺序是打乱的。
func (d *Dispatcher) PutJob(j Job) error {
	d.JobQueue <- j
	return nil
}

//Run 运行
func (d *Dispatcher) Run() {
	d.logger.Info("Run|调度守护启动……")
	workingNum := runtime.NumCPU() + 4
	check := time.NewTicker(d.dispatcherCheckDuration)
	var ch chan Job
	for {
		select {
		case <-check.C:
			//定时释放空闲的协程。
			n := len(d.workerPool)
			if n > workingNum {
				for i := workingNum; i < n; i++ {
					d.workerPool[i] <- nil
				}
				d.workerPool = d.workerPool[:workingNum]
			}
		case jobChannel := <-d.WorkerQueue:
			//超出池上限的协程释放。
			if len(d.workerPool) >= d.maxworkerPoolCount {
				//通知工作者关闭。
				jobChannel <- nil
			} else {
				d.workerPool = append(d.workerPool, jobChannel)
			}
			d.currentWorkersCount--
		case job := <-d.JobQueue:
			n := len(d.workerPool) - 1
			if n < 0 {
				//池中协程用完后，新建协程，牺牲内存抗峰值。
				worker := NewWorker(d)
				d.Wrap(worker.run)
				ch = worker.jobChan
			} else {
				ch = d.workerPool[n]
				d.workerPool[n] = nil
				d.workerPool = d.workerPool[:n]
			}
			d.currentWorkersCount++
			ch <- job
		case <-d.stopChan:
			goto end
		case <-d.Ctx.Done():
			d.Close()
		}
	}
end:
	ch = nil
	d.logger.Info("Run|等待子协程关闭。")
	for i := 0; i < len(d.workerPool); i++ {
		d.workerPool[i] <- nil
	}
	d.Wait()
	d.logger.Info("Run|调度守护关闭。")
	close(d.WorkerQueue)
	check.Stop()
	d.workerPool = nil
	//	close(d.JobQueue)
}

//Close 关闭
func (d *Dispatcher) Close() {
	d.closeOnce.Do(func() {
		close(d.stopChan)
	})
}
