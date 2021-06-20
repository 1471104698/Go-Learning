# 实现一个 goroutine pool



## pool 的作用

实际上学习了 Java 的线程池我们也知道线程池的作用是用来复用线程，减少线程的创建和销毁

实际上 Java 的线程模型是 内核级线程模型，即 1：1，一个 Java 线程对应一个 OS 线程，每次创建和销毁线程都需要深入到内核执行，所以性能开销大，因此需要线程池来管理这些线程

那么 Go 呢？goroutine 使用的是 两级线程模型，N:M 的对应关系，并不是每个 goroutine 都需要创建一个 OS 线程的，因此开销较小，同时 goroutine 本身就非常轻量级，每个 goroutine 初始的栈大小只有 2KB，并且会动态扩容/收缩，按照 GO 开发者的话 go 是能够支持成千上万的 goroutine 的

emmmm，平时也很少接触到这种并发量，按我本人来讲目前就实习了几个月左右，写的也只是一些几乎没什么并发量的业务代码，pool 没有太大用处，因此这里实现的 pool 在开发上来讲的话实际上并没有太大的作用，不过是想要来学习一些 GO 相关的一些知识，**学完用了才会印象深刻**。。。



## 前置知识

```
1、sync.Mutex(sync.Locker 的结构体实现，具有 Lock() 和 UnLock())
2、sync.WaitGroup
3、sync.Cond
4、atomic
5、如何控制 goroutine（因为 goroutine 并没有像 Java 那样对线程封装的 Thread 类）
```



## 1、sync.Mutex

sync.Mutex 类似 Java 中的 ReentrantLock，它实现了 sync.Locker 接口

内部维护了一个 state int32 变量，所谓的加锁和解锁都是对这个变量的操作，跟 ReentrantLock 的实现思想基本一致

```go
// A Locker represents an object that can be locked and unlocked.
type Locker interface {
	Lock()
	Unlock()
}

type Mutex struct {
	state int32
	sema  uint32
}

func (m *Mutex) Lock() {
	// Fast path: grab unlocked mutex.
	if atomic.CompareAndSwapInt32(&m.state, 0, mutexLocked) {
		if race.Enabled {
			race.Acquire(unsafe.Pointer(m))
		}
		return
	}
	// Slow path (outlined so that the fast path can be inlined)
	m.lockSlow()
}

func (m *Mutex) Unlock() {
	if race.Enabled {
		_ = m.state
		race.Release(unsafe.Pointer(m))
	}

	// Fast path: drop lock bit.
	new := atomic.AddInt32(&m.state, -mutexLocked)
	if new != 0 {
		// Outlined slow path to allow inlining the fast path.
		// To hide unlockSlow during tracing we skip one extra frame when tracing GoUnblock.
		m.unlockSlow(new)
	}
}
```



## 2、sync.WaitGroup

sync.WaitGroup 类似 Java 的 CountDownLatch，它是用来使某个 goroutine 等待其他 goroutine 执行完成后才继续执行下去，这段期间会进入阻塞状态

```go
type WaitGroup struct {
	noCopy noCopy

	// 64-bit value: high 32 bits are counter, low 32 bits are waiter count.
	// 64-bit atomic operations require 64-bit alignment, but 32-bit
	// compilers do not ensure it. So we allocate 12 bytes and then use
	// the aligned 8 bytes in them as state, and the other 4 as storage
	// for the sema.
	state1 [3]uint32
}

func (wg *WaitGroup) Add(delta int) 
func (wg *WaitGroup) Done()
func (wg *WaitGroup) Wait()
```

可以将 WaitGroup  理解为内部维护了一个计数器 state（内部操作使用 CAS），初始状态计数器值为 0，如果某个 goroutine 调用了 Wait()，那么如果该 goroutine 会进入阻塞状态，直到计数器的值为 0，该 goroutine 继续执行

（由此可以看来该 goroutine 不能先调用 Wait()，需要等待其他 goroutine 调用 Add()，否则一旦调用 Wait() 发现值为 0 了就直接继续执行了，起不到等待的作用）

被等待的线程在执行前调用 Add(1) 将计数器值 +1，在执行完成后调用 Done() 将计数器的值 -1，这样最终计数器的值会回归到 0 的状态，表示所有被等待的 goroutine 执行完成，这样的等待的 goroutine 会被唤醒，继续执行



## 3、sync.Cond

sync.Cond 类似 Java ReentrantLock 中的 Condition，用于生产者-消费者模式

```go
type Cond struct {
	noCopy noCopy

	// L is held while observing or changing the condition
	L Locker

	notify  notifyList
	checker copyChecker
}

// NewCond returns a new Cond with Locker l.
func NewCond(l Locker) *Cond {
	return &Cond{L: l}
}

func (c *Cond) Wait()
func (c *Cond) Signal()
func (c *Cond) Broadcast()
```

它内部维护了 一个 sync.Locker 和 一个等待队列，

它内部维护了三个 API：
1、Wait()：调用该 API 的 goroutine 会释放锁，然后进入到等待队列中处于自旋或者阻塞状态，等待其他 goroutine 的唤醒。（调用该 API 的前提是当前 goroutine 必须持有锁，即前面已经调用 sync.Locker 的 Lock() 获取锁成功了，否则报错（跟 Condition 一样）。）

2、Signal()：用于唤醒 cond 队列中的一个 goroutine 去获取锁，作用同 Java Condition 的 Signal()（跟 Condition 的区别在于调用该 API 的 goroutine **不需要获取锁**，即任意 goroutine 都可以调用该 API 去唤醒 cond 等待队列中的 goroutine）

3、Broadcast()：用于唤醒 cond 队列中的所有 goroutine 去争夺锁，此时可能出现虚假唤醒，所以注意使用 for 处理，作用同 Java Condition 的 SignalAll()（调用该 API 的 goroutine **不需要获取锁**）



## 4、atomic

pool 肯定会涉及到变量之类的原子性修改，比如 pool 的当前线程数以及线程的状态等，我们需要利用 atomic 进行 CAS

> #### Add

```go
// AddInt32 将 int32 类型的值加上某个值，这里添加是原子性的
// addr 是需要修改的值的指针，delta 是需要 add 的值，返回为修改后的值
func AddInt32(addr *int32, delta int32) (new int32)	

// AddUint32 同 AddInt32，只不过是 uint32，因此无法添加负值
func AddUint32(addr *uint32, delta uint32) (new uint32)

func AddInt64(addr *int64, delta int64) (new int64)

func AddUint64(addr *uint64, delta uint64) (new uint64)

func AddUintptr(addr *uintptr, delta uintptr) (new uintptr)
```



> #### Get

```go
// LoadInt32 获取 int32 类型的变量的值，这里获取是原子性的
// addr 是需要获取的 int32 变量的指针，返回结果为该变量的值
func LoadInt32(addr *int32) (val int32)

func LoadInt64(addr *int64) (val int64)

func LoadUint32(addr *uint32) (val uint32)

func LoadUint64(addr *uint64) (val uint64)

func LoadUintptr(addr *uintptr) (val uintptr)

func LoadPointer(addr *unsafe.Pointer) (val unsafe.Pointer)
```



> #### Set

```go
// StoreInt32 原子性地设置 int32 类型变量的值，名字可以理解为 SetInt32()
// addr 是待设置的 int32 变量的指针，val 是需要设置的新值
func StoreInt32(addr *int32, val int32)

func StoreInt64(addr *int64, val int64)

func StoreUint32(addr *uint32, val uint32)

func StoreUint64(addr *uint64, val uint64)

func StoreUintptr(addr *uintptr, val uintptr)
```



> #### Swap

```go
// SwapInt32 设置新值，返回旧值
// addr 是需要设置的 int32 变量的指针，new 是设置待设置的新值， 返回结果 old 是设置前的旧值
func SwapInt32(addr *int32, new int32) (old int32)

func SwapInt64(addr *int64, new int64) (old int64)

func SwapUint32(addr *uint32, new uint32) (old uint32)

func SwapUint64(addr *uint64, new uint64) (old uint64)

func SwapUintptr(addr *uintptr, new uintptr) (old uintptr)

func SwapPointer(addr *unsafe.Pointer, new unsafe.Pointer) (old unsafe.Pointer)
```



> #### CAS

```go
// CompareAndSwapInt32 CAS 实现
// addr 是需要设置的 int32 变量的指针， old 是旧值，new 是需要设置的新值，返回结果 swapped 表示当前 CAS 是否成功
func CompareAndSwapInt32(addr *int32, old, new int32) (swapped bool)

func CompareAndSwapInt64(addr *int64, old, new int64) (swapped bool)

func CompareAndSwapUint32(addr *uint32, old, new uint32) (swapped bool)

func CompareAndSwapUint64(addr *uint64, old, new uint64) (swapped bool)

func CompareAndSwapUintptr(addr *uintptr, old, new uintptr) (swapped bool)

func CompareAndSwapPointer(addr *unsafe.Pointer, old, new unsafe.Pointer) (swapped bool)
```





## 5、如何控制 goroutine

Java 的线程有一个抽象的结构体 Thread，我们可以通过操作 Thread 的 API 来操作这个线程，以此来控制 Thread 的生命周期

但是 Go  的线程单单只是对外暴露了一个关键字 `go`，没有提供一个抽象的实体，因此不能跟 Java 一样那样直接去控制

而 pool 实现的一个核心就是需要能够控制 goroutine，以此来复用 goroutine，减少 goroutine 的创建和销毁



因此我们需要借助一些特殊的数据结构来控制 goroutine，比如 channel、sync.Cond

这里我们使用 **sync.Cond**，它是一个不错的选择，我们可以维护一个全局的 sync.Cond，利用 cond.Wait() 来让没有任务可做的空闲的 goroutine 进入阻塞状态，等到存在任务的时候再调用 Signal() 将阻塞的 goroutine 唤醒去执行任务

这里也是模仿 Java ThreadPool 的一部分设计思想



## 6、pool 结构设计



> #### pool 结构
>
> 1、sync.Cond：用来控制 pool 阻塞等待
>
> 2、workers：跟 Java Pool 一样，维护一个 worker 列表，用于管理目前存在的 goroutine，将每个 goroutine 封装成一个 worker，不过 worker 内部并不是跟 Java 一样维护一个 Thread 实例，而是在 worker 内部自己开启一个 goroutine，控制 goroutine 的生命周期
>
> 3、cap：pool 的容量
>
> 4、maxSize：最大可存在的 worker 数
>
> 5、coreSize：核心 worker 数
>
> 6、runningSize：当前已经存在的 worker 数
>
> 7、panicHandler：异常处理，当发生 panic 时的处理策略
>
> 8、rejectHandler：类似 Java Pool 的 handler，当 pool 满时执行的拒绝策略
>
> 9、status：当前 pool 的状态，比如运行中、已关闭
>
> 10、lock：全局加锁
>
> 11、taskQueue：任务队列

```go
// pool
type pool struct {
	maxSize     int32
	coreSize    int32
	runningSize int32
	status      int32

	lock sync.Locker
	cond *sync.Cond

	// 这里实际上应该将 workers 做成一个接口，这样可以接收不同实现的任务队列
	workers   *workers
	taskQueue *taskQueue

	opts *Options
}

func (p *pool) Submit(task taskFunc) error
func (p *pool) Close()
func (p *pool) IsRunning() bool
func (p *pool) IsClosed() bool
func (p *pool) IsClosed() bool
func (p *pool) RunningSize() int32
func (p *pool) MaxSize() int32
func (p *pool) CoreSize() int32

func (p *pool) incrRunning(i int32)
func (p *pool) setStatus(status int32)
func (p *pool) addWorker(w *worker)
func (p *pool) getWorker(isFull isFullFunc) (w *worker)
func (p *pool) enTaskQueue(task taskFunc)
func (p *pool) deTaskQueue() (task taskFunc)

// Option
type Option func(*Options)

// Options pool 可选参数
type Options struct {
	// 是否预创建 worker
	isPreAllocation bool
	// 预创建的 worker 数
	allocationNum int32
	// panic 处理策略
	panicHandler PanicHandler
	// 拒绝策略
	rejectHandler RejectHandler
	// 当任务来临而没有 worker 可以创建，同时任务队列已满的时候是否阻塞当前 goroutine 等待出现空闲的 worker
	isBlocking bool
	// 最大的阻塞 goroutine 数
	blockMaxNum int32
	// 日志输出
	logger *log.Logger
}
```





> #### woker 结构
>
> 1、p：指向 pool，一是用来调用 pool 维护的 sync.Cond， 控制当前 worker 的 goroutine 的状态，二是用来判断 pool 的状态，比如是否已经关闭
>
> 2、task：当前 worker 需要执行的任务，以 chan 的形式接收
>
> 3、status：当前 worker 的状态

```go
// worker
type worker struct {
	p      *pool
	task   chan func()
	status int32
}
```



> #### workers 结构
>
> 使用队列的形式
>
> 1、cap：队列容量
>
> 2、len：队列内部元素个数
>
> 3、lock：用于并发加锁
>
> 4、producer：生产者
>
> 5、consumer：消费者
>
> 6、workers：容器

```go
// workers
type workers struct {
	cap int32
	len int32

	lock     sync.Locker
	producer *sync.Cond
	consumer *sync.Cond

	workers []*worker
}

func (ws *workers) Add(w *worker) error {}
func (ws *workers) Remove() (w *worker, err error) {}
func (ws *workers) Offer(w *worker) bool {}
func (ws *workers) Poll() (w *worker) {}
func (ws *workers) Put(w *worker) {}
func (ws *workers) Take() (w *worker) {}
func (ws *workers) enqueue(w *worker) {}
func (ws *workers) dequeue() (w *worker) {}
func (ws *workers) IsFull() {}
func (ws *workers) IsEmpty() bool {}
func (ws *workers) Reset() {}
```



> #### taskQueue 结构
>
> taskQueue 任务队列，实现基本跟 workers 一致，可惜目前没有泛型（虽然听说出了，不过还没用）
> 双向循环队列，头入队，尾出队，这里目前只支持一个 goroutine，如果要支持两个 goroutine，那么需要将 taskFunc 进行封装，每个内置一把 lock
>
> 1、cap：容量
>
> 2、len：当前存在的元素个数
>
> 3、head：头指针
>
> 4、tail：尾指针
>
> 5、lock：全局加锁
>
> 6、tasks：存储任务的容器

```go
type taskQueue struct {
	cap int32
	len int32

	head int32
	tail int32
	lock sync.Locker

	tasks []taskFunc
}
```



## 7、pool 任务大致处理逻辑

> #### 任务提交

pool 收到一个任务后，先判断 pool 是否已经关闭，

如果没有的话尝试从 workers 中获取空闲的 worker，获取成功直接将任务交给该 worker，获取失败的话那么判断 worker 数量是否已经到达 core，如果没有那么继续创建新的 worker，将任务交给该 worker

如果 worker 数量已经到达了 core，那么尝试将任务放到任务队列中，如果存放成功，那么直接返回，任务队列中的任务已经存在的 worker 会自动去获取

如果存放失败，那么再次尝试从 workers 中获取空闲的 worker，如果获取成功，任务交给该 worker 执行，获取失败，那么判断 worker 数是否到达 max，如果没有那么创建一个新的 worker，将该任务交给该 worker，如果已经到达了 max，那么执行拒绝策略



> #### worker 执行任务

worker 内部维护一个 chan，它接收的元素类型为需要执行的任务类型 fun()，在 worker 被创建后会调用它的 run()，该方法中会开一个 goroutine，不断阻塞等待从 chan 中获取任务，一旦获取到任务，那么直接执行。当任务执行完成后，它会再从任务队列中一直获取任务，如果任务队列中没有任务，那么它将作为空闲 worker 被存放到 workers 中等待其他任务的到来，此时它会继续阻塞在 chan 上

（goroutine 阻塞在 chan 上只会让 G 阻塞，而不会影响到 M，因此 M 是可以继续和其他的 G 进行绑定运行的，当收到任务时，G 被唤醒，寻找一个 P 的队列存储进去，等待 M 的调用）



## 8、目前版本的问题

这个 pool 是模仿 Java ThreadPool 的逻辑来实现的，不过目前版本并没有很好的使用 core 和 max，没有给 worker 设置空闲时间，意思就是一旦 worker 被创建那么在 pool 关闭前是不会被回收的，这实际上是一个很大的弊端，因为 core 和 max 形同虚设，后面再看看怎么去实现

