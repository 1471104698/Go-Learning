# slice



## slice 结构

```go
type slice struct {
	array unsafe.Pointer
	len int
	cap int
}
```

array 是指向数组基地址的指针，len 是数组内的元素个数，cap 是数组的容量

当声明一个 slice 类型的变量而没有初始化时，由于它本身是一个结构体，所以实际上已经分配好了内存结构，占用内存大小为 24B

```go
var sl []int
```

golang 判断 slice 是否为 nil 不是看结构体本身，结构体无法跟 nil 进行比较，它看的是 slice 内部的 array 指针，如果没有初始化，那么 array == nil，那么 sl == nil 为 true

```go
func main() {
	var sl []int
	fmt.Println(sl == nil) // true
    var sl2 []int = []int{}
    fmt.Println(sl2 == nil) // false
}
```



## slice 扩容原理

存在以下代码：

```go
func main() {
	s := []int32{1, 2}
	s = append(s, 3, 4, 5)
	fmt.Printf("len=%d, cap=%d", len(s), cap(s))
}
```

输出结果：

```
len=5, cap=8
```



扩容函数 growslice 代码如下：

```go
// old 是旧切片
// cap 是扩容后的最少需要的容量，上面 s 原 cap = 2，append 了 3 个元素，所以扩容后的最少容量 cap = 2 + 3 = 5
func growslice(et *_type, old slice, cap int) slice {
	newcap := old.cap
    // 旧切片容量翻倍
	doublecap := newcap + newcap
    // 如果最少需要的容量超过双倍容量，那么将新切片的容量设置为 cap
	if cap > doublecap {
		newcap = cap
	} else {
        // 如果双倍容量大于最少需要的容量，并且旧切片的容量小于 1024，那么将新切片的容量设置为双倍容量
		if old.len < 1024 {
			newcap = doublecap
		} else {
            // 旧切片的容量超过 1024，那么将新切片的容量一直增长 25%，直到大于最少需要的容量
			for newcap < cap {
				newcap += newcap / 4 // 每次增长 25%，直到大于最小值
			}
            // 容量溢出，那么直接将新切片的容量设置为最少需要的容量 cap
            if newcap <= 0 {
				newcap = cap
			}
		}
	}
}
```





## slice 传参坑点

> #### 坑点

```go
func main() {
    sl := make([]int, 0, 0)
    test(sl)
    fmt.Println(len(sl))
}

func test(sl []int) {
    sl = append(sl, 1)
}
```

输出结果：

```
0
```

因为初始创建的 sl 的 len == cap，所以在 test 函数中 append sl 会扩容，返回的是新的数据，而由于 goalng 是值传递，test 函数传入的是 外部 sl 的副本，内部的 array 也是外部 sl 的 array 的副本，只不过两个 array 指针指向的是同一个数组，但是 test 函数内部由于扩容而返回的新的切片，也就导致 test 函数内的副本的 array 指向了新的切片，然后再在新的切片上添加新的元素，这一操作不会影响到原 sl，所以外部的 sl 的 len 仍然是 0



> #### 解决方法：

当 slice 作为参数传递，并且函数内部可能需要对元素进行增删的话，那么最好是传 slice 的指针

```go
func main() {
    sl := make([]int, 0, 0)
    test(&sl)
    fmt.Println(len(sl))
}

func test(sl *[]int) {
    sl = append(sl, 1)
}
```

输出结果：

```
1
```

