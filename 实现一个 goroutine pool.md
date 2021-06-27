# 实现一个 goroutine pool



## pool 的作用

实际上学习了 Java 的线程池我们也知道线程池的作用是用来复用线程，减少线程的创建和销毁

Java 的线程模型是 内核级线程模型，即 **1：1**，一个 Java 线程对应一个 OS 线程，每次创建和销毁线程都需要深入到内核执行，所以性能开销大，因此需要线程池来管理这些线程

那么 Go 呢？goroutine 使用的是 两级线程模型，N：M 的对应关系，并不是每个 goroutine 都需要创建一个 OS 线程的，因此开销较小，同时 goroutine 本身就非常轻量级，每个 goroutine 初始的栈大小只有 2KB，并且会动态扩容/收缩，按照 GO 开发者的话 go 是能够支持成千上万的 goroutine 的

emmmm，平时也很少接触到高并发，按我本人来讲目前就实习了几个月左右，写的也只是一些几乎没什么并发量的业务代码，没有 pool  的应用场景



> #### 做 pool 的动机

在看 GMP 调度模型的时候，看到一篇文章讲完 GMP 后说实现一个 goroutine pool，看到我本身也是有兴趣的
因此打算根据它的部分设计思路+自身学习 Java ThreadPool 的源码逻辑来自己实现一个 goroutine pool
因此个人想尝试着手动实现一个 goroutine pool，它本身是比较考验和运用并发知识的，这对我个人而言的能力提升也是会有帮助的



## 前置知识

```go
1、sync.Mutex(sync.Locker 的结构体实现，具有 Lock() 和 UnLock())
2、sync.WaitGroup
3、sync.Cond
4、atomic
5、channel
6、time.Ticker、time.Timer
7、Go 调度模型 GMP
8、如何控制 goroutine（因为 goroutine 并没有像 Java 那样对线程封装的 Thread 类）
```



### 1、sync.Mutex

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



### 2、sync.WaitGroup

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



### 3、sync.Cond

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



### 4、atomic

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



### 5、chan

chan 全称为 channel，意为管道，它是 goroutine 通信的一种方式，同时也是**并发安全**的



> #### chan 的类型定义

```go
// 只能写入，不能读取
chan <- int

// 只能读取，不能写入
<- chan int

// 可写可读
chan int
```



> ####  chan 的创建

```go
// 创建一个容量为 1 的用于传输 int 元素的 chan，可写可读
ch := make(chan int, 1)

// 创建一个容量为 1 的用于传输 int 元素的 chan，只能写入
ch := make(chan <- int, 1)

// 创建一个容量为 1 的用于传输 int 元素的 chan，只能读取
ch := make(<- chan int, 1)

// 默认容量为 0，即阻塞队列，内部不能存储任何元素，一旦读或者写都会阻塞，直到另一个 goroutine 进行写或读
ch := make(chan int)
```



> ####  chan 的读取和写入阻塞情况

在 chan 的容量为 0 的情况下，那么读写都是阻塞的

```go
func main() {
	ch := make(chan int)
    go func() {
        <- ch
        fmt.Println("rece")
    }()
    time.Sleep(10 * time.Second)
    ch <- 1
}
```

上面我们使用 go 开了一个 子 goroutine，内部是使用 ch 读取数据，此时由于没有数据，因此它会阻塞在这里，而不会输出 ”rece“

在 主 goroutine 等待了 10s 后，往 ch 中塞入 1，子 goroutine 读取了这个 1，继续往下执行，打印 "rece"



在 chan 存在空余容量的情况下，那么写是非阻塞，如果内部没有数据，那么读是阻塞的

```go
// 写不阻塞
func test1() {
	go func() {
		time.Sleep(time.Hour)
	}()
	ch := make(chan int, 1)
	ch <- 1
}

// 读阻塞
func test2() {
	go func() {
		time.Sleep(time.Hour)
	}()
	ch := make(chan int, 1)
	<- ch
}
```

test1() 是 chan 内部有空余容量，因此写不会阻塞，无需等待 go func() 结束

test2() 是 chan 内部有空余容量，但是没有数据，因此读阻塞，需要等待 go func() 结束然后报 deadlock



> ####  chan deadlock 错误

```go
func main() {
	ch := make(chan int)
    go func() {
        fmt.Println("rece")
        time.Sleep(10 * time.Second)
    }()
    <- ch
}
```

主 goroutine 读取 ch 数据时陷入阻塞，子 goroutine 输出 "rece" 后会睡眠 10s，而睡眠完成后 子 goroutine 直接结束运行

这样的话理论上 主 goroutine 会无限期的陷入等待，但是实际上并不会：

- 当 主 goroutine 阻塞等待过程中发现其他所有的 goroutine 都已经结束了，那么意味着不会有其他 goroutine 给它的 ch 发送数据，因此此时直接报错 deadlock

输出结果：

```go
fatal error: all goroutines are asleep - deadlock!
```



> #### chan 的超时控制

```go
func main() {
    // 开启一个子 goroutine，避免 主 goroutine 发现没有其他 goroutine 存在而报 deadlock
    go func() {
        time.Sleep(time.Hour)
    }
	ch := make(chan int)
	ch2 := make(chan int)
	select {
		case i := <-ch:
			fmt.Println(i)
		case j := <-ch2:
			fmt.Println(j)
    }
}
```

这个例子利用 select 从 ch 和 ch2 中读取数据，主 goroutine 会在 select 阻塞等待直到 ch 或者 ch2 其中有一个接收到数据了然后执行其中一个分支，只要其他 goroutine 全部没有结束，并且没有数据传入，那么 主 goroutine 会一直等待下去

有时候我们并不想让它一直无限期的等待，因此我们需要加入超时控制



```go
func main() {
	ch := make(chan int)
	ch2 := make(chan int)
	select {
		case i := <-ch:
			fmt.Println(i)
		case j := <-ch2:
			fmt.Println(j)
        case <- time.After(2 * time.Second):
        	fmt.Println("timeout")
    }
}
```

这个例子中加入了一个 case，这个 case 是从 time.After(2 * time.Second) 中读取数据，time.After(2 * time.Second) 返回的是一个 chan Time，当超过指定的时间的时候，它会往这个 chan Time 上发送数据，以此使得第三个 case 被执行，结束 第一个 和 第二个 case 的无限期等待





### 6、time.Timer、time.Ticker

golang 的定时任务需要涉及到 time，而 time 提供了两种定时任务方式：

> #### time.Timer
>
> 通过 time.NewTimer() 创建一个 Timer，内部维护了一个 chan Time 类型的 C 字段，它会在过去时间段 d 后，向其自身的 C 字段发送当时的时间
>
> 只有一次触发

```go
type Timer struct {
	C <-chan Time
	r runtimeTimer
}

func NewTimer(d Duration) *Timer
```



> #### time.Ticker
>
> 通过 time.NewTimer() 返回一个新的 Ticker，该 Ticker 内部维护了一个 chan Time 类型的 C 字段，并会每隔时间段 d 就向该通道发送当时的时间。
>
> 多次触发，当不需要的时候需要调用 Stop() 停止定时任务

```go
type Ticker struct {
	C <-chan Time // The channel on which the ticks are delivered.
	r runtimeTimer
}

func NewTicker(d Duration) *Ticker
```



> #### 例子

```go
func main() {
    ticker := time.NewTicker(time.Second)
    for range ticker.C {
        fmt.Println("定时任务执行")
    }
}
```

我们开启了一个定时任务，时间间隔为 time.Second，即 1s，每隔 1s 都会进行执行 for 内部代码，打印输出



### 7、Go 调度模型-GMP

这个的话在这里进行了整理

https://github.com/1471104698/GoLearning/blob/master/GO%20%E8%B0%83%E5%BA%A6.md



### 8、如何控制 goroutine

Java 的线程有一个抽象的结构体 Thread，我们可以通过操作 Thread 的 API 来操作这个线程，以此来控制 Thread 的生命周期

但是 Go  的线程单单只是对外暴露了一个关键字 `go`，没有提供一个抽象的实体，因此不能跟 Java 一样那样直接去控制

而 pool 实现的一个核心就是需要能够控制 goroutine，以此来复用 goroutine，减少 goroutine 的创建和销毁



以下是计划：

```go
我们需要借助一些特殊的数据结构来控制 goroutine，比如 channel、sync.Cond

这里我们使用 chan，利用 chan 的阻塞特性来控制 goroutine，我们利用 chan 来实现任务的传递，goroutine 从 chan 中读取任务，如果没有任务，那么会阻塞，当收到任务后那么就开始执行，执行完成后继续阻塞等待。

这里实际上是让 goroutine 不退出 fun() 从而一直运行下去

同时 chan 的阻塞是用户态阻塞，只会阻塞 G，而不会影响到 M，因此 M 可以找其他 G 执行，成本小
```



## pool 结构设计（非最终结构）

### 1、pool 结构

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
> 7、freeTime：goroutine 最大空闲时间
>
> 8、panicHandler：异常处理，当发生 panic 时的处理策略
>
> 9、rejectHandler：类似 Java Pool 的 handler，当 pool 满时执行的拒绝策略
>
> 10、status：当前 pool 的状态，比如运行中、已关闭
>
> 11、lock：全局加锁
>
> 12、taskQueue：任务队列

```go
// pool
type pool struct {
	maxSize     int32
	coreSize    int32
	runningSize int32
	status      int32
	freeTime    int32

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



### 2、woker 结构

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

func (w *worker) run()
func (w *worker) getTask() (t taskFunc)
func (w *worker) isRecycle() bool
func (w *worker) setStatus(status int32)
func (w *worker) isNeedStop() bool
func (w *worker) IsRunning() bool
func (w *worker) IsStop() bool
```



### 3、workers 结构

> 存储 worker 的容器
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
func (ws *workers) checkWorker(i int32)
```



### 4、taskQueue 结构

> taskQueue 任务队列
>双向循环队列，头入队，尾出队，这里目前只支持一个 goroutine，如果要支持两个 goroutine，那么需要将 taskFunc 进行封装，每个内置一把 lock
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

func (q *taskQueue) Add(task taskFunc) bool
func (q *taskQueue) Poll() (task taskFunc)
func (q *taskQueue) PollWithTimeout(timeout int32, duration time.Duration) (task taskFunc)
func (q *taskQueue) enqueue(task taskFunc) bool
func (q *taskQueue) dequeue() (task taskFunc)
func (q *taskQueue) isFull() bool
func (q *taskQueue) isEmpty() bool
func (q *taskQueue) reset()
```



## pool 任务大致处理逻辑

### 1、任务提交

1、pool Submit() 一个任务，先判断 pool 是否已经关闭，没有的话继续往下执行

2、判断 worker 数是否已经达到 core，如果没有的话那么创建一个新的 worker，将任务交给它去执行

3、如果已经达到 core，那么尝试将任务提交到任务队列

4、如果提交成功，那么返回，如果提交失败，那么判断 worker 数是否已经达到 max，如果没有的话创建一个新的 worker，将任务交给它去执行

5、如果以上都无法成功，那么判断是否需要阻塞当前 goroutine

6、如果能够阻塞，那么超时阻塞等待，直到 worker 唤醒它

7、如果不能阻塞，那么执行拒绝策略



### 2、worker 执行任务

worker 内部维护一个 run()，该方法是任务执行的开始，它会开启一个 goroutine，在内部 for 死循环 不断调用 getTask() 获取、执行任务

当任务 t 执行完成后，会继续调用 getTask() 从任务队列中获取任务，此时任务的获取是有超时时间的，如果超时了，那么返回 nil

一旦 getTask() 没有获取到任务，表示当前任务并不多，不需要太多的 goroutine

因此判断当前线程是否需要进行回收，如果需要的话将当前 worker 的状态设置为 stop，并退出 run() 停止运行



### 3、stop worker 清理

每个 worker 处于 stop 状态时仍然是存在于 workers 队列中的，因此需要清除

在 pool 启动的时候就设置开启一个 goroutine 去执行定时任务，在定时任务中扫描 workers 部分 worker，如果是 stop worker 那么将它清除



### 4、pool 关闭

pool 关闭需要做以下几件事：

1、lock.Lock() 获取全局锁，防止 worker 的再创建

2、将 pool 状态设置为 Closed

3、清空任务队列

4、清空 workers 队列：将每个 worker 的状态设置为 stop，已经获取到任务或者正在执行会继续执行，执行完成后会自动退出，空闲的 worker 超时等待完成后会退出，将 workers 清空

5、关闭 chan

6、lock.Unlock() 释放锁



## pool 设计思路



### 1、worker 任务获取以及执行

> #### 初步设计

最开始没有设计任务队列以及超时控制，没有 core 和 max

 worker 的设计如下：

```go
type worker struct {
	p *pool
	task chan taskFunc
}

func (w *worker) run() {
    go func() {
        for t := range w.task {
            // 当接收到 nil，表示当前 worker 停止运行
            if t == nil {
                return
            }
            // 执行任务
            t()
            // 将当前 worker 存放到 workers 队列中等待调度
            w.p.addWorker(w)
        } 
    }()
}
```

worker 的执行逻辑如下：

1、通过 chan 阻塞来获取任务，空闲的时候阻塞在 chan 上

2、任务获取到后执行任务，执行完成后进入到 workers 队列中

3、继续执行等待调度

chan 阻塞并非是系统调用，它是用户态阻塞， G 会阻塞，不会导致 M 也阻塞， M 能够继续执行其他的 G，需要的也是用户态上下文切换，需要的成本不大



Submit() 的设计如下：

```go
// Submit 任务提交
func (p *pool) Submit(task taskFunc) error {
	// 判断 pool 是否已经关闭
	if p.IsClosed() {
		return poolClosedErr
	}

	// 获取 worker
	w := p.getWorker()

	// 这里拿到的 w 已经运行了 run()，直接塞任务即可
	w.task <- task
	return nil
}
```

此时设计的 workers 队列存储的是空闲等待的 worker，即上面的 worker 在执行完成任务后调用 addWorker() ，此时该 worker 就是空闲 worker

getWorker() 是尝试从 workers 中获取一个空闲的 worker，如果获取成功，那么直接返回，如果获取失败，那么判断是否能够创建一个新的 worker，如果能那么创建返回，同时调用 worker 的 run()

拿到 worker 后将任务通过 chan 传输给 worker `w.task <- task`，这里由于获取到的 w 必定是空闲的，即是阻塞在 chan 读上的，因此 Submit() 这里的 chan 写入不会产生阻塞



> #### 进一步设计

加入任务队列后， 当无法创建 worker 时，那么 Submit() 会将任务提交到任务队列上

此时 Submit() 的设计如下：

```go
// Submit 任务提交
func (p *pool) Submit(task taskFunc) error {
	// 判断 pool 是否已经关闭
	if p.IsClosed() {
		return poolClosedErr
	}
	// 获取 worker
	w := p.getWorker()
	if w == nil {
		// 将任务放到任务队列中
		if !p.enTaskQueue(task) {
            // 执行拒绝策略
            if r := p.opts.rejectHandler; r != nil {
                return r(task)
            }
            return poolFullErr
		}
		return nil
	}
    
	w.task <- task
	return nil
}
```

Submit() 执行逻辑如下：

1、先尝试获取一个 worker，获取成功直接将任务提交给它

2、获取失败，尝试将任务放到任务队列中，放入成功，返回

3、放入失败，执行拒绝策略



此时 worker 的 run() 设计如下：

```go
func (w *worker) run() {
    go func() {
        for t := range w.task {
            // 当接收到 nil，表示当前 worker 停止运行
            if t == nil {
                return
            }
            // 执行任务
            t()
            // 不断从任务队列中获取任务来执行
            for t = w.p.deTaskQueue(); t != nil; t = w.p.deTaskQueue() {
                t()
            }
            // 将当前 worker 存放到 workers 队列中等待调度
            w.p.addWorker(w)
        } 
    }()
}
```

worker 的 run() 执行逻辑如下：

1、先阻塞在 chan 上

2、等到从 chan 上获取到任务后，执行完成，然后 for 尝试从任务队列上获取任务，执行，执行无法获取到任务为止

3、任务队列无法获取到任务时，存储进 workers，继续作为空闲 worker 等待调用



### 2、worker 的空闲时间控制

> #### 初步设计

为了更好的控制 worker，因此设计加入了 freeTime、core、max，用来控制 worker 的数量，同时能够使得 worker 根据任务量自动调整

1、core：核心 worker 数

2、max：最大 worker 数，max >= core，超过 core 的 worker 数能够进行回收

3、freeTime：worker 的最大空闲时间



此时 worker run() 的设计如下：

```go
// run 执行任务
func (w *worker) run() {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := w.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if w.isRecycle() {
					w.setStatus(WorkerStop)
					return
				}
			} else {
				// 执行任务
				t()
                // 入队 workers，继续等待任务调度
				w.p.addWorker(w)
			}
		}
	}()
}

// getTask
func (w *worker) getTask() (t taskFunc) {
	// 利用 select 来完成超时控制
	select {
	case t = <- w.task:
		return t
	case <- time.After(time.Duration(w.freeTime) * time.Second):
	
	}
	// 尝试从任务队列中获取任务
	t = w.p.deTaskQueue(w.freeTime)
	if t != nil {
		return t
	}
	return nil
}
```

workrer run() 的执行逻辑如下：

1、for 循环调用 getTask() 获取任务

2、getTask() 中利用 select 对 chan 进行超时控制，此时超时的时间即为 worker 对应的 freeTime

3、如果等待的这段时间有 Submit() 有任务提交，那么停止阻塞，将任务返回

4、如果等待超时，那么尝试从任务队列中获取任务，如果获取成功，那么将任务返回

5、从 getTask() 返回后，判断是否获取到任务，如果获取到了那么执行任务，完成后入队 workers，继续等待调度

6、如果没有获取到任务，那么判断当前 worker 是否可以回收，如果可以的话那么直接回收，否则继续循环



这里的设计存在以下两个问题：

1、**执行 Submit() goroutine 发生阻塞** 

```go
getTask() 获取的任务有两种情况：
1、Submit() 提交到 chan，获取到 workers 中空闲的 worker 或者 创建新的 worker，将任务交给该 worker
2、从任务队列中获取

对于第一种情况，worker 执行任务没有什么问题，因为它此时是不存在于 workers 队列的，即表示它不是一个空闲 worker，那么在任务执行期间 Submit() 后面是不可能再拿到这个 worker 了，因此没问题。

对于第二种情况，worker 执行的任务不是由 Submit() 提交过来的，而是 worker 自己去找的，
并且可以看出它在拿到任务后也没有执行从 workers 中脱离的代码，
这也就意味着 worker 在执行这个任务的时候，它仍然是在 workers 队列中的，
那么下次 Submit() 是有可能把它当作空闲 worker，然后将任务提交给它，而此时它有任务在执行，那么此时 Submit() 就会阻塞住了
```

2、**发生 chan 方面的 deadlock**

```go
当 getTask() 超时等待从 select 代码出来，并且在任务队列中没有获取到任务，与此同时 Submit() 调用 getWorker() 获取到了这个 worker
这时候对于这个 worker 而言，它会判断自己是否需要进行回收，如果需要的话那么它会停止运行，直接 return，goroutine 结束
如果这时候 Submit() 调用 w.task <- task，发现对应持有该 task 的通道已经关闭了，那么它会报 deadlock（这是一个 fatal error，不是 panic，无法捕捉）
```



> #### 进一步设计

由于遇到这些问题，因此利用 **chan 来接收任务+select超时控制**  目前按照个人能力来说行不通

这里转变设计思路：

```go
1、worker 不再利用 chan 接收任务，直接从任务队列中获取任务，超时控制也存放在任务队列的获取中
2、Submit() 也不再使用 w.task <- task 传递任务，Submit() 只会创建新的 worker 或者将任务放到任务队列中，不会再从 workers 队列中获取 worker
3、workers 队列也不再存放空闲 worker，而是存放所有的队列，对于已经 stop 的 worker 利用 清理 goroutine 将它清除


（实际上这些想了挺久的，这个清理步骤最开始是被我忽略了的）
```



此时 worker run() 代码如下：

```go
// run 执行任务
func (w *worker) run() {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := w.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if w.isRecycle() {
					w.setStatus(WorkerStop)
					return
				}
			} else {
				// 执行任务
				t()
			}
		}
	}()
}

// getTask
func (w *worker) getTask() (t taskFunc) {
	if w.task != nil {
		t = w.task
		w.task = nil
		return t
	}
	// 尝试从任务队列中获取任务
	t = w.p.deTaskQueue(w.p.freeTime)
	if t != nil {
		return t
	}
	return nil
}

```

worker run() 执行逻辑如下：

1、调用 getTask() 获取任务

2、判断自身 task 是否存在任务，如果有的话直接执行，如果没有那么调用 任务队列超时获取任务

3、获取到任务后执行，如果没有获取到任务那么判断是否需要回收，需要的话设置状态为 stop，退出 goroutine



大体逻辑实际上并没有发生改变，修改的是任务队列的获取逻辑，让任务队列来完成超时等待获取的任务：

```go
// PollWithTimeout
func (q *taskQueue) PollWithTimeout(timeout int32, duration time.Duration) (task taskFunc) {
	endTime := time.Now().Add(time.Duration(timeout) * duration)
	for {
		if task = q.Poll(); task != nil {
			return task
		}
		remaining := endTime.Sub(time.Now())
		if remaining < 0 {
			break
		}
        // 等待添加时传入的信息
		select {
		case <-q.ch:
		case <-time.After(remaining):
		}
	}
	return nil
}

// enqueue 入队，调用该方法时必须获取锁
func (q *taskQueue) enqueue(task taskFunc) bool {
	if q.isFull() {
		return false
	}
	q.tasks[q.tail] = task
	q.tail = (q.tail + 1) % q.cap
	q.len++
    // 添加的时候传递信息，用于 PollWithTimeout，这里并不进行无限期的等待，超时等待
	select {
	case q.ch <- struct{}{}:
	case <-time.After(time.Millisecond):
	}
	return true
}
```



### 3、Submit() 线程超时阻塞等待

> #### 初步设计

用户在创建 pool 的时候可以选择当 worker 无法创建以及任务队列已满的情况下是否阻塞当前调用 Submit()  的 goroutine，等待唤醒然后将任务提交到任务队列中。

这里涉及到阻塞和唤醒，最开始的设计思路是使用 sync.Cond，它的作用跟 Java 的 Condition 一样，这算是典型的生产者消费者模型



Submit() 的设计如下：

```go
// Submit 任务提交
func (p *pool) Submit(task taskFunc) error {
	// 判断 pool 是否已经关闭
	if p.IsClosed() {
		return poolClosedErr
	}

	// 获取 worker 来执行任务
	w := p.getWorker(p.isCoreFull, task)
	// worker 数量达到了 core
	if w == nil {
		// 将任务放到任务队列中
		if !p.enTaskQueue(task) {
			// 任务队列已满，那么创建 非 core worker
			w = p.getWorker(p.isMaxFull, task)
			if w == nil {
				// 阻塞等待并存储到队列中
				if p.isNeedBlocking() && p.blockWait(task) {
					return nil
				}
				// 执行拒绝策略
				if r := p.opts.rejectHandler; r != nil {
					return r(task)
				}
				return poolFullErr
			}
		}
		return nil
	}
	return nil
}

// blockWait
func (p *pool) blockWait(task taskFunc) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.blockSize++
    for !p.enTaskQueue(task) && p.IsRunning() {
        // 阻塞等待唤醒
        p.cond.Wait()
    }
	p.blockSize--
	return true
}

```

Submit() 的执行逻辑如下：

1、先判断是否能够创建 core worker

2、不能的话再判断是否能够放入任务队列

3、不能的话再判断是否能够创建 非 core worker

4、都不能的话判断是否需要进行阻塞，如果需要的话那么调用 cond.Wait() 阻塞当前 goroutine，每次被唤醒都会尝试往任务队列中存放任务，如果失败了那么继续阻塞



唤醒的逻辑在 worker 上

worker 的设计如下：

```go
// run 执行任务
func (w *worker) run() {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := w.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if w.isRecycle() {
					w.setStatus(WorkerStop)
					return
				}

			} else {
				// 执行任务
				t()
				// 唤醒阻塞等待中的 Submit() goroutine
				if w.p.opts.isBlocking {
                    w.p.cond.Signal()
				}
			}
		}
	}()
}
```

worker 的执行逻辑如下：

1、尝试获取任务

2、获取到任务后执行任务，当执行完任务后调用 cond.Signal() 唤醒阻塞等待中的 goroutine

（此次唤醒对于 Submit() 处的 goroutine 可能是虚假唤醒，因为 t 可能是 worker 创建时携带过来的）



注意 Close() 的时候需要唤醒所有在等待的 goroutine：

```go
// Close
func (p *pool) Close() {
	// 获取锁，使得其他 goroutine 无法创建新的 worker
	p.lock.Lock()
	defer p.lock.Unlock()
	// 扫描所有的 workers 中所有的 worker，对于不在这里的 worker 在执行完任务后会自动退出
	p.setStatus(Closed)
	p.workers.reset()
	// 清空任务队列的任务
	p.taskQueue.reset()
	p.cond.Broadcast()
}
```



> #### 进一步设计

上面 Submit() 在阻塞的时候是没有限制等待的时间，即没有超时等待，因此这里加入一个超时等待的设计

单纯的 sync.Cond 实现不了超时等待，因此需要修改设计，使用 chan 来代替 sync.Cond

**利用 select + chan 来实现超时控制**



Submit() 的逻辑不变，只需要修改 blockWait() 的逻辑：

```go
// blockWait
func (p *pool) blockWait(task taskFunc) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.blockSize++
    // 计算超时结束时间
	endTime := time.Now().Add(p.opts.blockingTime)
    // 生产者-消费者模型
	for !p.enTaskQueue(task) && p.IsRunning() {
        // 计算剩下的超时时间
		remaining := endTime.Sub(time.Now())
        // 剩余时间 < 0，超时退出
		if remaining < 0 {
			return false
		}
        // select 两个 case，一个接收唤醒信号，一个用于超时限制
		select {
		case <- p.ch:
		case <- time.After(remaining):
		}
	}
	p.blockSize--
	return true
}
```



worker run() 的逻辑如下：

```go
// run 执行任务
func (w *worker) run() {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := w.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if w.isRecycle() {
					w.setStatus(WorkerStop)
					return
				}

			} else {
				// 执行任务
				t()
				// 唤醒阻塞等待中的 Submit() goroutine
                if w.p.opts.isBlocking {
                    select {
                        // 发送唤醒信号
                    case w.p.ch <- struct{}{}:	
                        // 超时等待
                    case <-time.After(time.Nanosecond):
                    }
                }
			}
		}
	}()
}
```

worker run() 执行逻辑如下：

1、当执行完任务后，会判断是否需要唤醒 Submit() goroutine

2、如果需要的话，那么使用 select 往 pool 里的 chan 发送唤醒信号，这里加入一个超时等待是因为可能此时 chan 已经满了，再写入会阻塞，那么这时候就不能一直死等，因此设置一个超时等待



目前超时等待的做法就想到这里

当 pool Close() 时不需要去管 Submit() 阻塞的 goroutine，当到达超时时间自动唤醒发现 pool 已经关闭时它们会自动退出





## pool 问题排查

### 问题1

#### 1、问题描述

在代码中最开始我将 workers 的容量设置为 maxSize，想着最多可以存储 maxSize 个 worker，但是实际上这是存在问题的
因为对于 stop worker 并不是立马就清除的，而是使用一个 clean goroutine 去执行，这样的话就会导致 实际正在运行的 worker 数小于 maxSize
因为 stop worker 在没有被清理前，在 Submit() 的视角里所有正在运行的 worker 数是小于 maxSize 的
因此它可以一个新的 worker，创建完成后它会将该 worker 存放到 workers 队列中
此时如果 stop worker + running worker 占满了整个队列，那么将会导致这一步阻塞，大大降低了执行效率



#### 2、解决

因此，现在默认将 workers 队列的 cap 设置为 math.MaxInt32（不提前进行分配）因为 workers 跟任务队列不同，它本身就不应该存在 worker 数限制，限制逻辑需要由 pool 自己去实现。
不过这实际上使得 workers 里面大部分的逻辑就变得空洞，比如 isFull()，目前仍做保留



### 问题2

#### 1、问题描述

有以下测试代码：

```go
var sum int32
func add(sum *int32, i int32) {
	time.Sleep(100 * time.Millisecond)
	atomic.AddInt32(sum, i)
	fmt.Println(i)
}

func main() {
	var sum int32
	var p = gpool.NewPool(100, 1000, 2, gpool.WithIsBlocking(true),
		gpool.WithIsPreAllocation(true))
	//times := 3000
	times := 10000
    for i := 0; i < times; i++ {
        // 每增加一个任务，那么 wg + 1
        wg.Add(1)
        j := i
        p.Submit(func() {
            add(&sum, int32(j))
            // 完成任务后 wg - 1
			wg.Done()
        })
    }
    // 阻塞等待任务完成
    wg.Wait()
    // 关闭 pool
    p.Close()
    except := getSum(times)
    fmt.Println("预期 sum 值：", except)
    fmt.Println("实际 sum 值:", sum)
    if sum != except {
        panic(fmt.Errorf("expect:%v, actually:%v", sum, except))
    }
}
```

两组测试数据：

1、core = 100, max = 1000，任务数为 10000

2、core = 1， max = 1000，任务数为 10000

测试数据 1 正常运行

测试数据 2 运行的过程中 main 阻塞不输出了

![image.png](https://pic.leetcode-cn.com/1624759551-ngzhXg-image.png)

后面多次运行，发现测试数据 1 都是正常运行，测试数据 2 问题必现



#### 2、排查思路

遇到阻塞第一反应就是发生了死锁，因此先跟着整个调用链路去盘点了一下整个获取锁的逻辑，不过整体看下来，所有的获取锁、释放锁和超时等待都没有太大的问题。

因此想着再打印 goroutine 栈来查看下栈内情况，发现 goroutine 并没有跟 Java jstack 那种比较方便的工具，不过后面看到了 pprof，作用差不多，在 main 程序启动的时候顺便启动 pprof，等到 main 阻塞，打开 pprof 查看 goroutine 栈情况：

![image.png](https://pic.leetcode-cn.com/1624760522-bdnKlL-image.png)

发现 goroutine 一直在任务队列的 PollWithTimeout 的 83 行代码这里运行，该行代码是 worker 超时获取任务的逻辑，chan + select

![image.png](https://pic.leetcode-cn.com/1624760683-jwCsjU-image.png)

这时候我认为是 worker 超时等待任务的逻辑出现了问题，虽然根据逻辑来说，它不应该会一直阻塞在 select，为了验证是否是这里出现的问题，加上一行输出语句：
![image.png](https://pic.leetcode-cn.com/1624760841-AByoqL-image.png)

再次执行 main 函数，发现控制台不断输出：

![image.png](https://pic.leetcode-cn.com/1624760938-mPRNat-image.png)

它并没有阻塞，但是程序没有结束，最终剩下一个 core goroutine 一直在超时获取任务，其他的 goroutine 都已经退出了

此时我将测试数据改成：core = 0， max = 1000，任务数为 10000

执行 main 函数，发现控制台输出阻塞：

![image.png](https://pic.leetcode-cn.com/1624761106-nnhaSn-image.png)

它这里显示 runningSize: 6 而没有显示 1 或者 0 是因为 core goroutine = 0，所有 goroutine 都已经完全退出，而不再继续超时获取任务，因此这里表示所有 goroutine 都是有正常退出的



#### 3、解决

任务提交、获取逻辑都排查过没有任何问题，并且 goroutine 都能够正常退出，那么问题应该出在哪里，这里实际上是困扰了我很久

后面想到 main() 线程阻塞住是因为 wg.Wait()，而在任务提交前就将 wg + 1，那么相当于 wg 的计数值为任务数，而只有在每个任务完成后 wg 的计数才会 -1，只有所有任务都能完成了 wg.Wait() 才会停止阻塞，这样的话是不是就意味着执行的过程中存在任务没有完成、或者任务被丢弃而导致 wg - 1 没有被执行的情况？

按照这个想法，我接收了 Submit() 返回的错误，同时输出了这个错误：

```go
var sum int32
func add(sum *int32, i int32) {
	time.Sleep(100 * time.Millisecond)
	atomic.AddInt32(sum, i)
	fmt.Println(i)
}

func main() {
	var sum int32
	var p = gpool.NewPool(100, 1000, 2, gpool.WithIsBlocking(true),
		gpool.WithIsPreAllocation(true))
	//times := 3000
	times := 10000
    for i := 0; i < times; i++ {
        // 每增加一个任务，那么 wg + 1
        wg.Add(1)
        j := i
        err := p.Submit(func() {
            add(&sum, int32(j))
            // 每执行完一个任务，那么 wg - 1
            wg.Done()
        })
        if err != nil {
            fmt.Printf("err：%v\n", err)
        }
    }
    // 阻塞等待任务完成
    wg.Wait()
    // 关闭 pool
    p.Close()
    except := getSum(times)
    fmt.Println("预期 sum 值：", except)
    fmt.Println("实际 sum 值:", sum)
    if sum != except {
        panic(fmt.Errorf("expect:%v, actually:%v", sum, except))
    }
}
```

发现的确是因为部分任务因为无法处理被丢弃而导致 wg - 1 无法被执行，从而导致 main() 阻塞：

![image.png](https://pic.leetcode-cn.com/1624761717-gEepRC-image.png)

任务被丢弃主要有以下几个原因：

1. 任务队列设置太小

2. core、max 数太小

3. Submit() 阻塞等待的时间太短了，只有 `10 * time.Nanosecond`，而我将每个任务的执行时间都设置在 `100 * time.Millisecond`，一旦超过阻塞等待的时间，那么 Submit() 将会执行拒绝策略，丢弃该任务

   ```go
   func add(sum *int32, i int32) {
       // 每个任务都在这里等待 100 * time.Millisecond
   	time.Sleep(100 * time.Millisecond)
   	atomic.AddInt32(sum, i)
   	fmt.Println(i)
   }
   ```



后续改进：为了防止这种问题而导致一直阻塞，在出现 err 的时候直接 **panic**，这样就能更容易定位问题

```go
func main() {
	var sum int32
	var p = gpool.NewPool2(0, 10, 10, gpool.WithIsBlocking(true),
		gpool.WithIsPreAllocation(false))
	//times := 3000
	times := 10000
    startTime := time.Now()
    sum = 0
    for i := 0; i < times; i++ {
        wg.Add(1)
        j := i
        err := p.Submit(p, func() {
            add(&sum, int32(j))
            wg.Done()
        })
        if err != nil {
            fmt.Printf("err：%v, 第 %v 个任务\n", err, i)
            // 这里直接 panic 终止进程，避免 wg.Wait() 阻塞
            panic(err)
        }
    }
    wg.Wait()
    fmt.Println("耗时：", time.Now().Sub(startTime))
    p.Close()
    except := getSum(times)
    fmt.Println("预期 sum 值：", except)
    fmt.Println("实际 sum 值:", sum)
    if sum != except {
        panic(fmt.Errorf("expect:%v, actually:%v", sum, except))
    }
}
```



### 问题3

#### 1、问题描述

在重新调整 worker 的设计的时候，想要将 worker 根据任务获取的不同方式而做成不同的 worker，这样的话它们就需要实现一个共同的接口，对外操作的就是这个接口方法

Worker 接口：

```go
// Worker worker 接口
type Worker interface {
	IsRunning() bool        // 当前 worker 是否在运行
	IsStop() bool           // 当前 worker 是否停止运行
	Close()                 // 关闭当前 worker
	run(Worker)             // 执行任务
	setTask(t TaskFunc)     // 设置任务，pool 不关心怎么将任务交给 worker，由 worker 自己去决定
	getTask() (t TaskFunc)  // 获取任务
	needExit() bool         // 当前 worker 是否需要退出
	notifyExit()            // 通知退出函数，因为不同类型的 worker 的退出逻辑是不一样的
	doAfter()               // doAfter 任务执行完成后进行的操作
	setStatus(status int32) // 设置状态
}
```

但是接口中的方法实际上对于不同的 worker 类型来说都是相同的逻辑，因此可以进行抽取

定义一个 worker 结构体来实现公共方法，类似 Java 的抽象类

worker 结构体：

```go
// worker 抽取封装的公共 worker
type worker struct {
	Worker
	p      GoroutinePool
	status int32
}

// 实现一些公共逻辑
func (w *worker) run() {
    
}

// 实现一些公共逻辑
func (w *worker) IsRunning() bool {
    
}
// .....
```

然后我们根据任务获取的类型对应的子类（实际上 go 并不存在子类的概念，这里只是为了方便理解按照 Java 方面的知识来讲）

每个子类内部都维护一个 *worker，这样就拥有了 *worker 对应的所有方法和变量

worker 子类：

```go
// workerWithQueue 从任务队列中获取任务的 worker
type workerWithQueue struct {
    // 内部维护 *worker
	*worker
	task TaskFunc
}

// workerWithChan 从 chan 中获取任务的 worker
type workerWithChan struct {
    // 内部维护 *worker
	*worker
	task chan TaskFunc
}
```

worker 实现的是共用方法，比如 run()，但是 run() 内部调用的并不全部都是公共的方法，有的是由子类实现的特殊逻辑

比如下面 run() 中，getTask()、needExit() 等都是需要由子类来实现的，worker 本身并没有实现这些方法

```go
func (w *worker) run() {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 对任务执行过程中发生的 panic 处理
		defer w.p.handlePanic()
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := w.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if w.needExit() {
					w.Close()
					w.p.decrRunning(1)
					return
				}
			} else {
				// 执行任务
				t()
				// 唤醒阻塞等待中的 Submit() goroutine
				w.doAfter()
			}
		}
	}()
}
```

在 pool 是创建一个 workerWithChan 来调用它的  run()，伪代码如下：

```go
func main() {
    workerSon := &workerWithChan{
        worker: newWorker(p),
        task:   make(chan TaskFunc),
    }
    workerSon.run()
}
```

然而当程序执行的时候，**报空指针异常 nil pointer**



#### 2、问题产生解析

这是因为 `workerSon.run()` 实际上是 `workerSon.worker.run()`，可以理解为这只是一个语法糖而已

因此在 run() 内部它所操作的 worker 实际上是 workerSon.worker，而不是 workerSon

因此内部调用的比如 getTask() 实际上并不是 workerSon的，而是 workerSon.worker 的

而此时由于 workerSon.worker 实际上并没有实现对应的逻辑，即对应 func() == nil，所以会报空指针异常。



这种设计跟 Java 的抽象类有很大的区别，虽然都是抽取共同的逻辑

在 Java 中实际上是存在父子类关系的，当子类实例调用某个方法的时候，如果子类没有实现该方法，那么它会自动找到父类的方法调用，而在父类方法中调用了其他方法，而这个方法子类重写了，那么它会调用子类的方法

在 Go 中并不存在什么父子类的概念，这种方式实际上只是相当于借用了 worker 去调用这些方法和变量，它们之间并没有什么特殊的关系



这里一个比较通俗易懂的例子：

> #### Go

```go
/*
	结构体 A 内嵌结构体 B，它们之间并不是什么父子关系，即不是 Java 的父子类
	这也就导致了当 结构体 B 实现了 eat() 和 say()，并且在 eat() 中调用了 say()，而结构体 A 也同样实现了 say()
	如果这时候使用 A 的实例去调用 eat()，那么由于 A 没有实现 eat()，所以调用的都是 B 的 eat()
	而在 B eat() 内部调用的 say() 也仍然是 B 的 say()，因为 A 和 B 本身并什么父子关系
	所以对 B 来说它就不会判断这是借助 A 调用过来的，不会去判断是否需要去查找 A 是否有 say() 然后调用它

	这个点算是跟 Java 不同的一个点之一
*/

// L 实现了 eat() 和 say()，在 eat() 内部调用了 say()
type L struct {
}

func (l *L) eat() {
	fmt.Println("eat L")
	l.say()
}

func (*L) say() {
	fmt.Println("say L")
}

// L1 内嵌了 L，并且实现了 say()
type L1 struct {
	*L
}

func (*L1) say() {
	fmt.Println("say L1")
}

func main() {
	l1 := &L1{
		L: &L{},
	}

	l1.eat()
}
```

输出：

```go
eat L
say L
```



> #### Java

```java
public class test {
    public static void main(String[] args) {
        B b = new B();
        b.eat();
    }
}

class A {
    public void eat() {
        System.out.println("A eat");
        say();
    }
    public void say() {
        System.out.println("A say");
    }
}

class B extends A {
    public void say() {
        System.out.println("B say");
    }
}
```

输出：

```java
A eat
B say
```



#### 3、解决

在 run() 方法参数中传入对应具体的 worker 子类 wi（方便理解）

对应的特殊逻辑都通过 wi 来调用，这种方法可能不是很好的解决方法，不过目前来说个人能力只能这么处理

```go
// run 执行任务
// 这里传入 wi Worker 的原因是因为我们外部创建的是 内嵌了 worker 的 workerWithQueue 或者 workerWithChan
// 但也只是相当于它们拥有了 worker 的变量和方法，这里我们使用 worker 来抽取出公共的逻辑，但是对于一些特殊的需要由 workerWithQueue 和 workerWithChan 的方法调用
// 由于 Go 的设计问题，我们不能直接使用 w 来进行调用，因为它并不会去识别这是 workerWithQueue 还是 workerWithChan，它会直接调用 worker 的方法
// 所以我们这里需要再传入对应的实现，然后使用该实现去调用这些特殊的方法
func (w *worker) run(wi Worker) {
	w.setStatus(WorkerRunning)
	// 开启一个 goroutine 执行任务
	go func() {
		// 对任务执行过程中发生的 panic 处理
		defer w.p.handlePanic()
		// 阻塞接收任务，chan 阻塞的是 G，不会影响到 M，M 仍然可以继续去跟其他的 G 进行绑定
		for {
			t := wi.getTask()
			if t == nil {
				// 如果当前 worker 需要回收，那么结束运行
				if wi.needExit() {
					wi.Close()
					w.p.decrRunning(1)
					return
				}
			} else {
				// 执行任务
				t()
				// 唤醒阻塞等待中的 Submit() goroutine
				wi.doAfter()
			}
		}
	}()
}
```

