package main

func main() {
	// 前述
	/*
		nil 是什么？类型？变量？关键字？
		根据 golang 官方源码，可以看出，nil 是一个 int 类型的变量
		以下是官方源码：
		```go
			// nil is a predeclared identifier representing the zero value for a
			// pointer, channel, func, interface, map, or slice type.
			var nil Type // Type must be a pointer, channel, func, interface, map, or slice type

			// Type is here for the purposes of documentation only. It is a stand-in
			// for any Go type, but represents the same type for any given function
			// invocation.
			type Type int
		```
		但是，实际上我们并不能将 nil 当作一个 int 型的变量，我们需要将 nil 当作一个符号
		编译器在出现 nil 的地方会进行特殊的判断，它不会把它当作 int 型变量去处理，而是单独存在一个 if 分支：
		```go
			if(出现 nil) {
				特殊处理
			}
		```
	*/

	// golang 变量内存分配
	/*
		golang 跟 C 变量分配到的内存情况是存在差异的：
			1、golang 是在分配内存时会保证“置0分配”，即最终分配给变量的内存上的数据都必定是全0的，在分配内存时会将内存上的数据全部置0
			2、C 分配到的内存上的数据是未知的，可能全0，可能全1，可能是0101，是由上次分配使用遗留下来的数据，对于当前次分配来说该数据无效
		golang 是在语言层面就已经保证分配的内存是全0的了
	*/

	// nil 的意义
	/*
		什么样的数据类型可以使用 nil？
		除了基本数据类型（int、float32、bool、byte、rune、string）外都可以使用 nil
		有 6 种数据类型：slice、map、chan、interface{}、ptr、struct

		我们不能赋值给一个 int 值为 nil，因为这是编译器所不允许的，仅仅是编译器不允许而已
		我们可以把 nil 当作一个触发条件，在出现 nil 的时候，编译器会对变量的类型进行判断，如果是基本数据类型，那么会报错
		1、如果是跟 nil 进行比较，那么会先判断是否是以上 6 种数据类型
		2、如果是 nil 赋值，那么会先判断是否是以上 6 种数据类型，然后再将对应的变量内存值置0
	*/

	// nil 判断 和 赋值
	/*
		slice:
			比较：slice 本体是一个 24 byte 的复合结构体，nil 判断是只判断 data 数组指针内存值是否全0，与 cap 和 len 无关
			赋值：sl = nil 实际上是将 24 byte 内存地址全置0
		map:
			比较：map 本体是一个 8 byte 的指针，nil 判断是只判断指针内存地址是否全0，即是否有指向 hmap 结构体
			赋值：m = nil 实际上是将 8 byte 指针的内存地址全置0
		chan：
			类似 map
		interface{}：
			interface{} 是一个复合结构体，它有两种数据类型：
			```go
				type iface struct {
					tab  *itab
					data unsafe.Pointer
				}
				type eface struct {
					_type *_type
					data  unsafe.Pointer
				}
			```
			第一种 iface 实际上是我们平常定义的接口对应存储的结构体
			第二种 eface 实际上是我们平常讲的空接口 interface{}
			它们本质上都是一个复合结构体，内部包含两个指针，占用 16 byte

			比较：判断 data 指针是否为 nil
			赋值：将 16 byte 都置0
		指针：
			```go
				var ptr *int
			```
			指针本身就是一个 8 byte 的内存地址而已

			比较：判断 8 byte 是否为 nil
			赋值：将 8 byte 置0
	*/
}
