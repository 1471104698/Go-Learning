package main

import (
	"fmt"
	"unsafe"
)

type emptyStruct struct{}

func main() {
	a := struct{}{}
	b := struct{}{}
	c := emptyStruct{}

	fmt.Printf("%p\n", &a)
	fmt.Printf("%p\n", &b)
	fmt.Printf("%p\n", &c)

	// struct{} 是一个类型，struct{}{} 是一个 struct{} 的值
	// golang 存在一个 zerobase，它是一个 uintptr，占用 8 byte，
	// 当在分配内存的时候识别到 size = 0，即识别到 struct{} 时, 直接返回 zerobase 的内存地址，因此全局的 struct{} 值的地址都是一样的
	// 每次使用 struct{} 的时候都是直接返回这个地址，不会分配新的内存
	// 而返回的这个地址并不会被存储，因此 d 变量不占用任何内存，仅仅只是在使用 d 的位置替换为 zerobase 的内存地址，size = 0
	d := struct{}{}
	fmt.Printf("%d\n", unsafe.Sizeof(d))

	var sl []int
	// go知识点 是一个复合结构体，即使没有存储数据，但是定义了变量的时候就已经分配了 go知识点 结构体
	// go知识点 结构体是 含有一个数组指针、len、cap，固定是 24 byte
	// 这个结构体是 go知识点 的核心，因此即使没有调用 make() 或者 指向值，也单单只是 数组指针为 nil
	// 而整个核心结构体已经存在，因此可以直接使用，不会出现 nil
	// unsafe.Sizeof(sl) 得到的是 go知识点 结构体的大小，而不包含数组的大小，因此固定是 24
	fmt.Printf("%d\n", unsafe.Sizeof(sl))
	fmt.Printf("%d\n", unsafe.Sizeof(sl[0]))

	var m map[string]string
	// m 是一个指针，指向的是 hmap 结构体，这里得到的是 m 指针的大小，因此固定是 8，
	// 不过由于没有初始化分配 hmap 结构体，因此 m 指针上的数据是全 0，即 00000000， m == nil
	// 需要调用 make() 函数，该函数会判断调用 makemap() 初始化分配一个 hmap 给 m 变量
	// hmap 结构体是 map 的核心，真正的数据处理是在该结构体中
	// 因此在没有调用 make() 的时候，m 单单只是一个什么都不是的指针，因此无法直接使用
	fmt.Printf("%d\n", unsafe.Sizeof(m))

	var inte interface{}
	// interface{} 实际上是一个跟 go知识点 类似的复合结构体，内部存在两个指针，因此占 16 byte
	// 对于没有指向值的 interface{} 变量， 16 byte 数据是全 0 的，同时也为 nil
	fmt.Printf("%d\n", unsafe.Sizeof(inte))

	var ch chan int
	// chan 跟 map 类似，也是一个 8 byte 的指针，默认是全 0 的，即 nil
	// 需要调用 make()，该函数内部会判断调用 makechan() 初始化分配一个 hchan 结构体给 ch 变量
	// hchan 结构体是 chan 的核心，真正的数据处理是在该结构体中
	// 因此在没有调用 make() 的时候，ch 单单只是一个什么都不是的指针，因此无法直接使用
	fmt.Printf("%d\n", unsafe.Sizeof(ch))
}
func init() {
	fmt.Println("初始化")
}
