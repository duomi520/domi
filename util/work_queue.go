package util

import (
	"runtime/debug"
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
	defer func() {
		if r := recover(); r != nil {
			w.dispatcher.logger.Error("Worker|异常拦截：", r, string(debug.Stack()))
		}
		w.dispatcher.workerPoolCount--
	}()
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
	JobQueue chan Job //任务队列
	jobSlice []Job    //任务切片

	workerQueue     chan chan Job //空闲工作者队列
	workerPool      []chan Job    //空闲工作者池
	workerPoolCount int           //池的协程数

	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
	logger    *Logger
	WaitGroupWrapper
}

//NewDispatcher 新建
func NewDispatcher(count int) *Dispatcher {
	logger, _ := NewLogger(ErrorLevel, "")
	logger.SetMark("Dispatcher")
	d := &Dispatcher{
		JobQueue:        make(chan Job, MaxQueue),
		jobSlice:        make([]Job, 0, MaxQueue),
		workerQueue:     make(chan chan Job, count),
		workerPool:      make([]chan Job, count),
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
	for i := 0; i < d.workerPoolCount; i++ {
		worker := NewWorker(d)
		d.workerPool[i] = worker.jobChan
		d.Wrap(worker.run)
	}
	for {
		//多路复用，运行任务的顺序是打乱的。
		select {
		case jj := <-d.workerQueue:
			d.workerPool = append(d.workerPool, jj)
		case j := <-d.JobQueue:
			d.jobSlice = append(d.jobSlice, j)
		case <-snippet.C:
			d.assignmentTask()
		case <-d.stopChan:
			d.logger.Debug("Run|等待工作者关闭……")
			snippet.Stop()
			for len(d.jobSlice) > 0 {
				d.assignmentTask()
				time.Sleep(10 * time.Microsecond)
			}
			for wp := range d.workerPool {
				d.workerPool[wp] <- nil
				d.workerPoolCount--
			}
			for d.workerPoolCount > 0 {
				wq := <-d.workerQueue
				wq <- nil
				d.workerPoolCount--
			}
			d.Wait()
			d.logger.Debug("Run|调度守护关闭。")
			d.jobSlice = nil
			d.workerPool = nil
			close(d.workerQueue)
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

func (d *Dispatcher) assignmentTask() {
	js := len(d.jobSlice)
	if js > 0 {
		wp := len(d.workerPool)
		if js >= wp {
			for i := 0; i < wp; i++ {
				d.workerPool[i] <- d.jobSlice[i]
			}
			d.jobSlice = d.jobSlice[wp:]
			d.workerPool = d.workerPool[:0]
		} else {
			for i := 0; i < js; i++ {
				d.workerPool[wp-1-i] <- d.jobSlice[i]
			}
			d.jobSlice = d.jobSlice[:0]
			d.workerPool = d.workerPool[:wp-js]
		}
	}
}
