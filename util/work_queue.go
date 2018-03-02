package util

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

//MaxQueue 队列最大缓存数
const MaxQueue = 2048

//ErrDispatcherClose 调度者已关闭
var ErrDispatcherClose = errors.New("util.PutJob|调度者已关闭。")

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
	Ctx  context.Context
	Name string

	JobQueue    chan Job      //任务队列
	workerQueue chan chan Job //空闲工作者队列

	DispatcherCheckDuration time.Duration //释放闲置的协程的时间间隔
	maxworkerPoolCount      int           //池最大的协程数
	stopChan                chan struct{} //退出信号
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
		logger:                  logger,
	}
	return d
}

//PutJob 入队操作
//运行一个任务 多路复用，运行的顺序是打乱的。
func (d *Dispatcher) PutJob(j Job) error {
	select {
	case <-d.stopChan:
		return ErrDispatcherClose
	default:
		d.JobQueue <- j
		return nil
	}
}

//Run 运行
func (d *Dispatcher) Run() {
	d.logger.Info("Run|调度守护启动……")
	check := time.NewTicker(d.DispatcherCheckDuration)
	minimumWorker := runtime.NumCPU() + 8
	//初始化空闲工作者池
	workerPool := make([]chan Job, d.maxworkerPoolCount)
	for i := 0; i < d.maxworkerPoolCount; i++ {
		worker := NewWorker(d)
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
				worker := NewWorker(d)
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
			d.logger.Info("Run|等待工作者关闭。")
			check.Stop()
			for ix := range workerPool {
				workerPool[ix] <- nil
			}
			go func() {
				j := <-d.workerQueue
				j <- nil
			}()
			d.Wait()
			d.logger.Info("Run|调度守护关闭。")
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
