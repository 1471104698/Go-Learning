package main

import (
	"fmt"
)

/*
	内存可见性问题：golang 存在多个现场 goroutine，同时每个 CPU 都存在 L1、L2、L3 cache
	因此对于 CPU 调用来说，golang 跟 Java 一样都存在内存可见性问题
*/

var a int
var ch = make(chan int, 3)

func main() {
	// 对于该代码，按照线程执行顺序，肯定是先执行完 a = 1 才会开一个线程去执行 go fun()
	// 因此 1 的代码执行必定是对 3 可见的，因为它保证了执行顺序，因此能够输出 1
	a = 1       // 1
	go func() { // 2
		println(a) // 3
	}()

	// 对于该代码，按照线程执行顺序，由于是先开一个线程，然后主从线程一起执行，我们无法保证 1 一定会比 2 先执行
	// 因此 1 的代码未必对 3 可见，因为我们无法保证执行顺序，可能先执行 1 也可能先执行 2，
	// 因此输出的可能是 0，也可能是 2
	go func() {
		a = 2 // 1
	}()
	fmt.Println(a) // 2

	// 对于该代码，按照线程执行顺序，虽然先开了一个线程，但是 3 存在 <- ch 阻塞，它需要等待 2 执行完
	// 而由于按照执行顺序， 1 先于 2 执行完，3 先于 4 执行完，因此 1 先于 4 执行完
	// 因此 1 的代码对 4 来说是可见，因此输出 2
	go func() {
		a = 2   // 1
		ch <- 1 // 2
	}()
	<-ch           // 3
	fmt.Println(a) // 4
}

// 我们可以利用 chan 特性以及设置对应的容量来控制并发执行的线程数
func do() {
	work := []func(){
		func() {},
		func() {},
		func() {},
		func() {},
	}
	for _, w := range work {
		go func(w func()) {
			ch <- 1
			w()
			<-ch
		}(w)
	}
}
