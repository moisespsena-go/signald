package signald

import (
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	defaultlogger "github.com/moisespsena-go/default-logger"
	"github.com/moisespsena-go/logging"
	path_helpers "github.com/moisespsena-go/path-helpers"
)

var (
	KillSignals   = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	RestartSignal os.Signal

	log = defaultlogger.GetOrCreateLogger(path_helpers.GetCalledDir())
)

type Monitor struct {
	Log       logging.Logger
	callbacks []struct {
		ptr uintptr
		f   func(os.Signal)
	}
	mu, cbmu,
	donemu,
	restartsmu sync.RWMutex
	started        bool
	done, restarts []func(os.Signal)
	c              chan os.Signal
	doneC          chan interface{}
	restartSignal  os.Signal
}

func (this *Monitor) Restartable() {
	this.restartsmu.Lock()
	defer this.restartsmu.Unlock()
	this.restartSignal = RestartSignal
}

func (this *Monitor) Restarts(f func(sig os.Signal)) {
	this.restartsmu.Lock()
	defer this.restartsmu.Unlock()
	this.restarts = append(this.restarts, f)
}

func (this *Monitor) Done(f func(sig os.Signal)) {
	this.donemu.Lock()
	defer this.donemu.Unlock()
	this.done = append(this.done, f)
}

func (this *Monitor) Bind(f func(sig os.Signal)) *Monitor {
	this.cbmu.Lock()
	defer this.cbmu.Unlock()
	s := struct {
		ptr uintptr
		f   func(os.Signal)
	}{reflect.ValueOf(f).Pointer(), f}
	this.callbacks = append(this.callbacks, s)
	return this
}

func (this *Monitor) Unbind(f func(os.Signal)) *Monitor {
	this.cbmu.Lock()
	defer this.cbmu.Unlock()
	ptr := reflect.ValueOf(f).Pointer()
	for i, cb := range this.callbacks {
		if cb.ptr == ptr {
			this.callbacks = append(this.callbacks[0:i], this.callbacks[i+1:]...)
			return this
		}
	}
	return this
}

func (this *Monitor) AutoBind(binder Binder) *Monitor {
	this.Bind(binder.Callback)
	this.Start()
	if binder.Unbind != nil {
		binder.Unbind(func() {
			this.Unbind(binder.Callback)
		})
	}
	return this
}

func (this *Monitor) Stop() {
	close(this.c)
}

func (this *Monitor) Start(sig ...os.Signal) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.started {
		return
	}
	this.started = true
	if this.Log == nil {
		this.Log = log
	}
	if len(sig) == 0 {
		sig = append(sig, append(KillSignals, RestartSignal)...)
	}
	this.doneC = make(chan interface{})
	this.c = make(chan os.Signal, 1)

	signal.Notify(this.c, sig...)
	go func() {
		var sig os.Signal
		for sig = range this.c {
			this.Log.Notice("received signal:", sig.String())

			if sig == this.restartSignal {
				func() {
					this.restartsmu.RLock()
					defer this.restartsmu.RUnlock()
					for _, cb := range this.restarts {
						cb(sig)
					}
				}()
				continue
			}

			func() {
				this.cbmu.RLock()
				defer this.cbmu.RUnlock()
				for _, cb := range this.callbacks {
					cb.f(sig)
				}
			}()

			for _, s := range KillSignals {
				if s != sig {
					continue
				}
				go func() {
					this.donemu.Lock()
					defer this.donemu.Unlock()
					for _, cb := range this.done {
						cb(sig)
					}
					close(this.doneC)
				}()
				return
			}
		}
	}()
}

func (this *Monitor) Wait() {
	if this.doneC != nil {
		<-this.doneC
	}
}

var m Monitor

func Done(f ...func(sig os.Signal)) {
	for _, f := range f {
		m.Done(f)
	}
}
func Restarts(f ...func(sig os.Signal)) {
	for _, f := range f {
		m.Restarts(f)
	}
}

func Bind(f ...func(os.Signal)) *Monitor {
	for _, f := range f {
		m.Bind(f)
	}
	return &m
}

func Unbind(f ...func(os.Signal)) *Monitor {
	for _, f := range f {
		m.Unbind(f)
	}
	return &m
}

func Start() {
	m.Start()
}

func Restartable() {
	m.Restartable()
}

type BinderInterface interface {
	SignalBinder() Binder
}

func AutoBindInterface(binder BinderInterface) *Monitor {
	return m.AutoBind(binder.SignalBinder())
}

func AutoBind(binder Binder) *Monitor {
	return m.AutoBind(binder)
}

type Binder struct {
	Callback func(os.Signal)
	Unbind   func(f func())
}

func Stop() {
	m.Stop()
}

func Wait() {
	m.Start()
	m.Wait()
}
