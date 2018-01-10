package util

import (
	"context"
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
	dispatcherWorkStopChan chan struct{}
	dispatcherWorkerQueue  chan chan Job
	jobChan                chan Job
}

//NewWorker 新建
func NewWorker(s chan struct{}, w chan chan Job) *Worker {
	return &Worker{
		dispatcherWorkStopChan: s,
		dispatcherWorkerQueue:  w,
		jobChan:                make(chan Job),
	}
}

//run 运行
func (w *Worker) run() {
	for {
		select {
		case j := <-w.jobChan:
			if j == nil {
				goto end
			}
			j.WorkFunc()
			w.dispatcherWorkerQueue <- w.jobChan
		case <-w.dispatcherWorkStopChan:
			goto end
		}
	}
end:
	w.dispatcherWorkStopChan = nil
	w.dispatcherWorkerQueue = nil
	close(w.jobChan)
}

//Dispatcher 调度者,控制io发送
type Dispatcher struct {
	Ctx  context.Context
	Name string

	JobQueue    chan Job      //任务队列
	workerQueue chan chan Job //空闲工作者队列

	DispatcherCheckDuration time.Duration //释放闲置的协程的时间间隔
	maxworkerPoolCount      int           //池最大的协程数
	stopChan, workStopChan  chan struct{} //退出信号
	closeOnce               sync.Once
	logger                  *Logger
	WaitGroupWrapper
}

//NewDispatcher 新建
func NewDispatcher(ctx context.Context, name string, maxCount int) *Dispatcher {
	logger, _ := NewLogger(DebugLevel, "")
	logger.SetMark("Dispatcher." + name)
	d := &Dispatcher{
		Ctx:  ctx,
		Name: name,

		JobQueue:    make(chan Job, MaxQueue),
		workerQueue: make(chan chan Job, MaxQueue),

		DispatcherCheckDuration: 5 * time.Second,
		maxworkerPoolCount:      maxCount,
		stopChan:                make(chan struct{}),
		workStopChan:            make(chan struct{}),
		logger:                  logger,
	}
	return d
}

//Run 运行
func (d *Dispatcher) Run() {
	d.logger.Info("Run|调度守护启动……")
	check := time.NewTicker(d.DispatcherCheckDuration)
	minimumWorker := runtime.NumCPU() + 8
	//初始化空闲工作者池
	workerPool := make([]chan Job, d.maxworkerPoolCount)
	for i := 0; i < d.maxworkerPoolCount; i++ {
		worker := NewWorker(d.workStopChan, d.workerQueue)
		workerPool[i] = worker.jobChan
		d.Wrap(worker.run)
	}
	for {
		select {
		case jobChannel := <-d.workerQueue:
			//超出池上限的协程释放。
			if len(workerPool) >= d.maxworkerPoolCount {
				//通知工作者关闭。
				jobChannel <- nil
			} else {
				//放入池。
				workerPool = append(workerPool, jobChannel)
			}
		//多路复用，运行任务的顺序是打乱的。
		case job := <-d.JobQueue:
			n := len(workerPool) - 1
			if n < 0 {
				//池中协程用完后，新建协程，牺牲内存抗峰值。
				worker := NewWorker(d.workStopChan, d.workerQueue)
				d.Wrap(worker.run)
				worker.jobChan <- job
			} else {
				workerPool[n] <- job
				workerPool[n] = nil
				workerPool = workerPool[:n]
			}

		case <-check.C:
			//定时释放空闲的协程。
			n := len(workerPool)
			if n > minimumWorker {
				for i := minimumWorker; i < n; i++ {
					workerPool[i] <- nil
				}
				workerPool = workerPool[:minimumWorker]
			}
		case <-d.Ctx.Done():
			d.Close()
		case <-d.stopChan:
			goto end
		}
	}
end:
	d.logger.Info("Run|等待工作者关闭。")
	check.Stop()
	close(d.workStopChan)
	d.Wait()
	d.logger.Info("Run|调度守护关闭。")
	//close(d.workerQueue)
	//close(d.JobQueue)
}

//Close 关闭
func (d *Dispatcher) Close() {
	d.closeOnce.Do(func() {
		close(d.stopChan)
	})
}
