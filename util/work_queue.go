package util

import (
	"runtime"
	"sync"
	"time"
)

//MaxQueue 队列最大缓存数
const MaxQueue = 2048

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
			close(w.jobChan)
			return
		}
		j.WorkFunc()
		w.dispatcher.workerQueue <- w.jobChan
	}
}

//Dispatcher 调度者,控制io发送
type Dispatcher struct {
	Name string

	JobQueue    chan Job      //任务队列
	workerQueue chan chan Job //空闲工作者队列
	jobSlice    []Job         //任务切片

	DispatcherCheckDuration time.Duration //释放闲置的协程的时间间隔
	workingCount            int           //
	maxworkerPoolCount      int           //池最大的协程数

	stopChan chan struct{} //退出信号

	closeOnce sync.Once
	logger    *Logger
	WaitGroupWrapper
}

//NewDispatcher 新建
func NewDispatcher(name string, maxCount int) *Dispatcher {
	logger, _ := NewLogger(ErrorLevel, "")
	logger.SetMark("Dispatcher." + name)
	d := &Dispatcher{
		Name: name,

		JobQueue:    make(chan Job, MaxQueue),
		workerQueue: make(chan chan Job, MaxQueue),
		jobSlice:    make([]Job, 0, MaxQueue),

		DispatcherCheckDuration: 5 * time.Second,
		maxworkerPoolCount:      maxCount,
		stopChan:                make(chan struct{}),
		logger:                  logger,
	}
	return d
}

//PutJob 入队操作
//运行一个任务 多路复用，运行的顺序是打乱的。
func (d *Dispatcher) PutJob(j Job) error {
	d.JobQueue <- j
	return nil
}

//Run 运行
func (d *Dispatcher) Run() {
	d.logger.Debug("Run|调度守护启动……")
	check := time.NewTicker(d.DispatcherCheckDuration)
	minimumWorker := runtime.NumCPU() + 8
	if d.maxworkerPoolCount < minimumWorker {
		d.maxworkerPoolCount = minimumWorker
	}
	//初始化空闲工作者池
	workerPool := make([]chan Job, minimumWorker, d.maxworkerPoolCount)
	d.workingCount = minimumWorker
	for i := 0; i < minimumWorker; i++ {
		worker := NewWorker(d)
		workerPool[i] = worker.jobChan
		d.Wrap(worker.run)
	}
	for {
		select {
		case jobChannel := <-d.workerQueue:
			if len(d.jobSlice) > 0 {
				jobChannel <- d.jobSlice[0]
				d.jobSlice = d.jobSlice[1:]
			} else {
				//超出池上限的协程释放。
				if len(workerPool) >= d.maxworkerPoolCount {
					//通知工作者关闭。
					jobChannel <- nil
					d.workingCount--
				} else {
					//放入池。
					workerPool = append(workerPool, jobChannel)
				}
			}

		//多路复用，运行任务的顺序是打乱的。
		case job := <-d.JobQueue:
			if d.workingCount <= d.maxworkerPoolCount {
				n := len(workerPool) - 1
				if n < 0 {
					worker := NewWorker(d)
					d.Wrap(worker.run)
					d.workingCount++
					worker.jobChan <- job
				} else {
					workerPool[n] <- job
					workerPool[n] = nil
					workerPool = workerPool[:n]
				}
			} else {
				d.jobSlice = append(d.jobSlice, job)
			}
		case <-check.C:
			//定时释放空闲的协程。
			n := len(workerPool)
			if n > minimumWorker {
				for i := minimumWorker; i < n; i++ {
					workerPool[i] <- nil
					d.workingCount--
				}
				workerPool = workerPool[:minimumWorker]
			}
		case <-d.stopChan:
			d.logger.Debug("Run|等待工作者关闭……")
			check.Stop()
			for ix := range workerPool {
				workerPool[ix] <- nil
			}
			go func() {
				j := <-d.workerQueue
				j <- nil
			}()
			d.Wait()
			d.logger.Debug("Run|调度守护关闭。")
			return
		}
	}
}

//Close 关闭
func (d *Dispatcher) Close() {
	d.closeOnce.Do(func() {
		close(d.stopChan)
	})
}
