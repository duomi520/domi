package util

import (
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

	workerPoolCount int //池的协程数

	stopChan chan struct{} //退出信号

	closeOnce sync.Once
	logger    *Logger
	WaitGroupWrapper
}

//NewDispatcher 新建
func NewDispatcher(name string, count int) *Dispatcher {
	logger, _ := NewLogger(ErrorLevel, "")
	logger.SetMark("Dispatcher." + name)
	d := &Dispatcher{
		Name: name,

		JobQueue:    make(chan Job, MaxQueue),
		workerQueue: make(chan chan Job, count),
		jobSlice:    make([]Job, 0, MaxQueue),

		workerPoolCount: count,
		stopChan:        make(chan struct{}),
		logger:          logger,
	}
	return d
}

//Run 运行
func (d *Dispatcher) Run() {
	d.logger.Debug("Run|调度守护启动……")
	snippet := time.NewTicker(10 * time.Microsecond)
	//初始化空闲工作者池
	workerPool := make([]chan Job, d.workerPoolCount)
	for i := 0; i < d.workerPoolCount; i++ {
		worker := NewWorker(d)
		workerPool[i] = worker.jobChan
		d.Wrap(worker.run)
	}
	for {
		//多路复用，运行任务的顺序是打乱的。
		select {
		case jobChannel := <-d.workerQueue:
			//放入池。
			workerPool = append(workerPool, jobChannel)
		case <-snippet.C:
			js := len(d.jobSlice)
			if js > 0 {
				wp := len(workerPool)
				if js >= wp {
					for i := 0; i < wp; i++ {
						workerPool[i] <- d.jobSlice[i]
					}
					d.jobSlice = d.jobSlice[wp:]
					workerPool = workerPool[:0]
				} else {
					for i := 0; i < js; i++ {
						workerPool[wp-1-i] <- d.jobSlice[i]
					}
					d.jobSlice = d.jobSlice[:0]
					workerPool = workerPool[:wp-js]
				}
			}
		case j := <-d.JobQueue:
			d.jobSlice = append(d.jobSlice, j)
		case <-d.stopChan:
			d.logger.Debug("Run|等待工作者关闭……")
			snippet.Stop()
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
