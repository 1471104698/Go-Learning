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
4、如何控制 goroutine（因为 goroutine 并没有像 Java 那样对线程封装的 Thread 类）
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



## 4、如何控制 goroutine

Java 的线程有一个抽象的结构体 Thread，我们可以通过操作 Thread 的 API 来操作这个线程，以此来控制 Thread 的生命周期

但是 Go  的线程单单只是对外暴露了一个关键字 `go`，没有提供一个抽象的实体，因此不能跟 Java 一样那样直接去控制

而 pool 实现的一个核心就能够控制 goroutine，以此来复用 goroutine，减少 goroutine 的创建和销毁