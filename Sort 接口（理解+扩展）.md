# Sort 接口



## 1、接触 Sort 接口前-自述

最开始实习，学习 golang，会了基本语法后，在公司内部做的一个小小的业务需要涉及到排序

当时由于个人学习不深，加上看到 golang 并没有提供基础的 栈、队列等数据结构的实现，同时在学习的过程中没有接触到 sort 包，认为 golang 没有提供 sort 的实现

```
现在看来这种认知就存在问题，sort 作为一个非常常用的 API，这种语言一般不会没有提供实现，所以应该先去 google，现在也没理解为什么当时没有去 google
不过当时没有 google 在现在看来也是一个挺不错的选择，后面才能够引发我自己的思考
```

然后就自己想着自己去实现一个通用的排序方法，也是想重新温习下堆排序，所以使用堆排来作为排序算法的实现

最开始排序数组的类型是我业务上使用的结构体类型，根据记忆当时定义的排序结构大概如下：

```go
// User 待排序结构体
type User struct {
	ID int32
    Age int32
    Name string
}

// HeapSort 封装排序逻辑
type HeapSort struct {
    // 待排序数据
    users []User
    // 比较器，判断是否需要交换，由用户实现传入
    comparator func(u1, u2 User) bool
}

// Sort 对外提供的排序接口
func (hs *HeapSort) Sort() {
    // ...
}

// buildHeap 初始建队
func (hs *HeapSort) buildHeap() {
 	// ...   
}

// heapify 调整堆
func (hs *HeapSort) heapify(i, len int) {
 	// ...
    c1 := i*2+1 
    c2 := i*2+2
    max := i
    if c1 < len && hs.compare(hs.users[max], hs.users[c1]) {
        max = c1
    }
    if c2 < len && hs.compare(hs.users[max], hs.users[c2]) {
        max = c2
    }
}
```

HeapSort 内部维护待排序的数组，以及一个比较器，实现大体上的排序逻辑，用户只需要传入待排序数据以及对应的比较逻辑即可

emmm，这个目前适用我需要排序的结构体 User 了

但是不是通用的，因此进一步设计，将 HeapSort数据修改成所有数据通用的类型：

```go
// HeapSort 封装排序逻辑
type HeapSort struct {
    // 待排序数据
    data []interface{}
    // 比较器，判断是否需要交换，由用户实现传入
    comparator func(u1, u2 interface{}) bool
}

// Sort 对外提供的排序接口
func (hs *HeapSort) Sort() {
    // ...
}

// buildHeap 初始建队
func (hs *HeapSort) buildHeap() {
 	// ...   
}

// heapify 调整堆
func (hs *HeapSort) heapify(i, len int) {
    c1 := i*2+1 
    c2 := i*2+2
    max := i
    if c1 < len && hs.compare(hs.data[max], hs.data[c1]) {
        max = c1
    }
    if c2 < len && hs.compare(hs.data[max], hs.data[c2]) {
        max = c2
    }
    // ...
}
```

但实际上这样是有问题的，这种实现方式可以看出写出这个代码的我本身实际上是不够熟悉 Go 语言的，思路层面上还停留在刚从 Java 转 Go 而还没有从 Java 的语法上跳出来

我将 interface{} 理解为 Java 的 Object，理所当然地认为将 []interface{} 就类似 []Object

interface{} 作为参数可以 接收任意值，那么 []interface{} 理论上作为参数可以接收任意参数类型的 slice

但实际上这是错误的，golang 实际上并不会把 []interface{} 当作 slice 方面的 interface{}

我们可以理解为 interface{} 类型 golang 在编译的时候会进行特殊处理，作为参数的时候接受任意类型都编译/运行通过，而 []interface{} 类型作为参数的时候并没有进行特殊处理，它就是把它当作一个 []interface{}，就跟当作 []int、[]string  这种来处理，不会进行特殊处理，因此 []interface{} 和 []int 之间不能直接进行转换

如果要将 []int 转换为 []interface{} 的话，那么需要遍历 []int 的所有元素，然后使用一个 []interface 来一个个接收

例子：

```go
func main() {
	is := []int{1, 2, 3}
	test(is)				// 编译报错
	
	var its []interface{}
	its = is				// 编译报错
	its = []interface(is)	// 编译报错
	for _, v := range is {
		its = append(its, v)
	}
	test(its)				// 编译成功且运行成功
}

func test(its []interface{}) {

}
```



综上，**这种 []interface{} 的做法本身就是有问题的，Java 的做法不适合 Go。**

后面也想了挺久的，有想过每次都进行类型强转，但是对于一些用户自定义类型是很难进行转换的，无法写死，后面就想用 reflect 反射来获取数据的类型 Type，然后再根据这个 Type 进行类型转换，但是可惜的是 Go 跟 Java 不同，`reflect.TypeOf()` 拿到的 Type 里面的所有信息都不能直接使用 `i.(Type)`

因此直接放弃这条路了，本来以为 Go 没有泛型的存在基本不太可能实现通用的排序逻辑的时候，google 了下发现 Sort 接口的存在，也就是这样，才真正找到了 Go 的语法方面的一些诀窍



## 2、Sort 接口

在 sort 包中，提供了一个命名为 Interface 的接口，接口内部有 3 个方法：

```go
package sort

// Interface 调用 sort 必须实现的接口
type Interface interface {
    // 获取排序数组长度
	Len() int
    // 比较逻辑，i 和 j 是索引值，并且 i > j，判断后面的 i 是否需要交换到 j 前面
    // （实际上这里的索引名设置有点难理解，因为一般情况下按照习惯都是认为 i < j 的，但看方法内部就是按照 i > j 的方式来传参）
    // 即如果我们要实现升序排序，那么越小的值排在越前面，此时存在 i > j 的两个索引位置值的比较，即 i 的位置是排在 j 后面的
    // 我们需要判断 i 是否需要交换到 j 前面，让 j 往后面靠
    // 如果需要交换的话那么 nums[i] 的值必须比 nums[j] 小，这样后面的小值才能交换到前面的大值前面
    // 因此这里 Less(i, j int) bool 逻辑为 return nums[i] < nums[j]
	Less(i, j int) bool
    // 交换两个元素的逻辑
	Swap(i, j int)
}

// Sort 对外提供的排序接口
func Sort(data Interface) {
	n := data.Len()
	quickSort(data, 0, n, maxDepth(n))
}
```

我们可以看出，sort 包内部的 Sort() 方法它的接收参数是 Interface 类型的，Sort() 内部实现了基本的排序逻辑，不过涉及到 len 获取、compare 逻辑、元素交换逻辑 的时候是调用传入的 data 的 Len()、Less()、Swap()

这意味着我们如果要实现一个数组的排序，那么我们只需要实现 Interface 接口的三个方法，实现好 len 获取、compare 逻辑、元素交换逻辑 即可，其他的排序逻辑都直接丢给 sort.Sort() 去实现

它这种设计绕过了 []interface{} 类型的限制，利用接口实现通用化，只要实现该接口的都可以直接使用 sort.Sort()，只要抽取不同排序数组不同逻辑的几个方法做成接口让用户自己去实现即可，sort.Sort() 内部不关心如何实现。

那么对于 []byte 这种不是结构体的，不能实现接口方法的怎么办？

将他封装为一个结构体就可以了：

```go
// 注意这里不能使用 type Bs = []byte，这种别名无法实现方法
type Bs []byte

func (bs *Byte) Len() {
	return len(*bs)
}

func (bs *Byte) Less(i, j int) bool {
	return (*bs)[i] < (*bs)[j]
}

func (bs *Byte) Swap(i, j int) {
	(*bs)[i], (*bs)[j] = (*bs)[j], (*bs)[i]
}
```

实际上看到这里，再看我上面的设计，我被 Java 困得有点深，加上基本没遇到过这种设计，想不到利用接口的方式来代替 []interface{}

这也算是一次不错的学习



## 3、Sort 接口的扩展：Heap

golang 也是提供了 heap 堆的接口，我们可以用来实现大顶堆和小顶堆：

```go
package heap

import "sort"

type Interface interface {
	sort.Interface
	Push(x interface{})
	Pop() interface{}
}

// 初始化数据，构建初始堆，如果 h 没有数据，那么该方法没有意义
func Init(h Interface) {}
// 添加一个数据，然后重新调整堆
func Push(h Interface, x interface{}) {}
// 弹出一个数据，然后重新调整堆
func Pop(h Interface) interface{} {}
// 移除某个位置的数据，然后重新调整堆
func Remove(h Interface, i int) interface{} {}
```

可以看出，heap 包下也定义了一个 Interface 接口，内部引用了 sort.Interface 以及自己定义的两个接口方法 Push() 和 Pop()

因此 heap.Interface 接口的方法汇总如下：

```go
type Interface interface {
    Len() int			// 获取长度
    Less(i, j int) bool	// 比较逻辑
    Swap(i, j int)		// 交换逻辑
    Push(x interface{})	// push 逻辑
    Pop() interface{}	// pop 逻辑
}
```

heap 包下定义的几个方法接收的也都是 Interface 接口类型，因此这个做法实际上是跟 sort.Sort() 是一样的，我们只需要让我们要进行建堆的数据实现这几个接口，然后直接调用 heap.Init()、heap.Push() 和 heap.Pop() 等进行建堆

而要实现的是大顶堆还是小顶堆，那么就跟我们实现的 Less() 逻辑相关了



下面是大顶堆和小顶堆的各自实现：

```go
// 利用 Heap 实现共用逻辑，类似隐式的父类
type Heap []int

func (h *Heap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *Heap) Len() int {
	return len(*h)
}

// Pop 数组末尾元素
// 注意这里为什么是 Pop 数组末尾元素，一般情况下顶堆 Pop() 的值应该是在堆顶，即 nums[0]，但是我们这里是借 heap.Pop() 来进行调用的
// 因此我们需要看 heap.Pop() 的逻辑，它是先将 nums[0] 和 nums[len-1] 进行交换，再调整堆结构，再调用这里的 Pop()
// 即它将堆顶元素放到了数组末尾，所以我们这里 Pop() 的应该是数组末尾的元素
func (h *Heap) Pop() (v interface{}) {
	*h, v = (*h)[:len(*h)-1], (*h)[len(*h)-1]
	return v
}

// Push 在数组末尾
// heap.Push() 是先调用这里的 Push()，然后从数据末尾索引进行堆调整，因此直接放在数组末尾没毛病
func (h *Heap) Push(v interface{}) {
	*h = append(*h, v.(int))
}

func (h *Heap) Peek() int {
	return (*h)[0]
}

// MaxHeap 大顶堆
type MaxHeap struct {
    // 内部维护一个 Heap 即可使用 Heap 的方法
	Heap
}

// Less() 表示 i 是否需要排在 j 前面，这里的 i > j，因为 i 在 j 后面，所以这里判断是否需要将 i 交换到 j 前面
// 我们这里是大顶堆，那么越大的值越排在前面，那么如果排在后面的 i 满足 nums[i] > nums[j]，那么 i 需要交换到前面
func (h *MaxHeap) Less(i, j int) bool {
	return h.Heap[i] > h.Heap[j]
}

// MinHeap 小顶堆
type MinHeap struct {
    // 内部维护一个 Heap 即可使用 Heap 的方法
	Heap
}

// Less() 表示 i 是否需要排在 j 前面，这里的 i > j，因为 i 在 j 后面，所以这里判断是否需要将 i 交换到 j 前面
// 我们这里是小顶堆，那么越小的值越排在前面，那么如果排在后面的 i 满足 nums[i] < nums[j]，那么 i 需要交换到前面
func (h *MinHeap) Less(i, j int) bool {
	return h.Heap[i] < h.Heap[j]
}

```

