# Golang 反射



## 1、反射的作用

反射用来获取到对象的变量和方法，能够在运行过程中获取对象的状态，并将修改或者调用它们

一般情况下，在编写代码的时候，我们自己定义的结构体肯定知道它有什么样的变量和方法，能够进行特定的处理。但是对于程序代码来说，如果传入的是 interface{} 之类的类型，它们并不知道它拥有什么样的变量和方法，因此，这时候我们就可以利用反射窥探它的元数据，获取它的变量和方法，然后根据这些信息进行特定的处理，比如判断是否存在某个变量，如果存在就做什么样的处理。。。



一般反射的作用：

1、获取元数据

2、热更新：利用反射在运行过程中修改配置信息，避免服务重启



## 2、reflect 

在 golang reflect 标准库中，最重要的两个结构体为：`reflect.Type` 和 `reflect.Value`，反射的所有方法围绕的就是这两个结构体

- `reflect.Type` 用于存储对象的类型信息，比如对象名、数据类型、持有的所有方法 Type、变量 Type

- `reflect.Value` 用于存储对象的值信息，比如所有的方法 Value、变量 Value

利用 `reflect.TypeOf()` 获取 `Type`，`reflect.ValueOf()` 获取 `Value`

![图片](https://mmbiz.qpic.cn/mmbiz_jpg/KVl0giak5ib4ia0pMqtgQmAXya4gWfYQ28Qic0tgGde8Hk2ZianbJZgVnYOXet7ofNUSqj2eFEzQtBffE8d9EHTj5qQ/640?wx_fmt=jpeg&tp=webp&wxfrom=5&wx_lazy=1&wx_co=1)

### 2.1、reflect.Type

> #### Type 是一个接口，内部方法如下

```golang
type Type interface {
     // 适用于所有类型
     // 返回该类型内存对齐后所占用的字节数
     Align() int

     // 返回该类型的方法集中的第 i 个方法
     Method(int) Method

     // 根据方法名获取对应方法集中的方法，无法获取私有方法
     MethodByName(string) (Method, bool)

     // 返回该类型的方法集中导出的方法的数量，注意是可导出的，私有方法无法获取
     NumMethod() int

    // 获取第 i 个变量信息（编译期间就已经将所有的变量都计算好存储为元数据了，类似 Java 已经做成一个模板了）
     Field(i int) StructField

	 // 嵌套获取某个变量信息
	 FieldByIndex(index []int) StructField

	 // 根据变量名获取变量信息
	 FieldByName(name string) (StructField, bool)
	
	 // 获取变量的个数
	 NumField() int
    
     // 返回该类型的名称，比如 People、a
     Name() string

     // 获取所在的包名
     PkgPath() string
    
    // 返回 "PkgPath().Name()"
     String() string

     // 返回该类型的类型，这个类型是笼统的，比如 int、struct、func
     Kind()	Kind
    
	 ...
}
```



> #### 例子

```go
type People struct {
	a int
}

func (People) Eat() {
    
}

func main() {
	t := reflect.TypeOf(People{1})
	fmt.Println(t.Kind())		// struct
	fmt.Println(t.String())		// main.People
	fmt.Println(t.Name())		// People
    fmt.Println(t.PkgPath())	// main
	
    // 获取所有变量信息
    for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)			// i = 0，获取第 0 个变量 a
		fmt.Println(field.Name)		// a，变量名
		fmt.Println(field.Type)		// 0，偏移量，可以用于 CAS
		fmt.Println(field.Offset)	// main， 包名，跟所在结构体一致
        fmt.Println(field.Index)	// 0，当前变量在结构体元数据中的索引位置
	}
    
    // 获取所有可导出的方法信息，不包含私有方法
    for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		fmt.Println(method.Name)	// 方法名
		fmt.Println(method.Func)	// 方法的 Value，可以用于反射执行该方法
		fmt.Println(method.Index)	// 方法在结构体元数据中的索引位置
	}
}
```



### 2.2、reflect.Value

> ####  `reflect.Value` 结构

```go
type Value struct {
    // 对象的 Type
	typ *rtype
    
    // 指向反射对象的指针
	ptr unsafe.Pointer
    
    // 当前值是否支持修改
    flag 
}
```



可以看到，`reflect.Value` 的结构比较简单，它内部维护了一个 *rtype 变量，这个类型实现了 `reflect.Type`  的所有方法，因此可以理解为 `reflect.Value` 内部维护了 `reflect.Type`，即我们可以通过 `reflect.Value` 获取 `reflect.Type`

这样的话，实际上 `reflect.Value` 也拥有 `reflect.Type` 的所有方法，不过 `reflect.Value` 会对这些方法进行一些修改，因为 `reflect.Value` 是用来获取值的，而 `reflect.Value` 的所有方法都是用来获取信息的，因此 `reflect.Value` 的这些方法返回都是 Value



> #### `reflect.Value` 和 `reflect.Type` 的协作关系

```go
func (v Value) Field(i int) Value {
	if v.kind() != Struct {
		panic(&ValueError{"reflect.Value.Field", v.kind()})
	}
	tt := (*structType)(unsafe.Pointer(v.typ))
	if uint(i) >= uint(len(tt.fields)) {
		panic("reflect: Field index out of range")
	}
	field := &tt.fields[i]
	typ := field.typ

	// Inherit permission bits from v, but clear flagEmbedRO.
	fl := v.flag&(flagStickyRO|flagIndir|flagAddr) | flag(typ.Kind())
	// Using an unexported field forces flagRO.
	if !field.name.isExported() {
		if field.embedded() {
			fl |= flagEmbedRO
		} else {
			fl |= flagStickyRO
		}
	}
    // 根据数据指针 ptr 和 变量的偏移量获取变量的真实数据
	ptr := add(v.ptr, field.offset(), "same as non-reflect &v.field")
	return Value{typ, ptr, fl}
}
```

它通过内部维护的 rtype 获取变量的信息，然后根据 ptr 数据指针和变量offset 获取变量的真实数据，然后封装成一个 Value 返回

其他的方法都是类似的做法



> #### `reflect.Value` 特有的方法

```go
// 修改变量的值
func (Value) Set(v Value)

// 获取指针指向的值
func (Value) Elem() Value

// 将 Value 内部值转换为 interface{} 类型
func (Value) Interface() interface{}

// 将 Value 内的值转换为 int64 返回
func (Value) Int() int64

// 将 Value 内的值转换为 float64 返回
func (Value) Float() float64

// 如果 Value 内的值是 string，那么转换为 string 输出，如果不是，那么返回 Kind()
func (Value) String() string

func (Value) SetInt(x int64)

// 当前 Value 是否能够调用 Set() 进行修改，内部是对 flag 和 指针之类的判断
func (Value) CanSet() bool

// Value 内部值为函数时，利用该方法去反射调用该函数，入参和出参都是 []reflect.Value，如果没有入参，那么为 nil，如果没有出参，那么为 nil
func (Value) Call(in []Value) []Value
```



> #### CanSet() 方法

该方法跟 Value 的 flag 挂钩，根据 flag 来判断是否支持修改

```go
func (v Value) CanSet() bool {
	return v.flag&(flagAddr|flagRO) == flagAddr
}
```



## 3、获取、修改和调用

### 3.1、获取变量

```go
func main() {
	a := 100
	// 获取 Value
	v := reflect.ValueOf(a)
	fmt.Println(v.Interface())			//100
	fmt.Println(v.Interface().(int))	//100
	fmt.Println(v.Int())				//100
	
    fmt.Println(v.CanSet())				//false
	v.SetInt(50)	//报错，panic: reflect: reflect.Value.SetInt using unaddressable value
}

```



> #### 问题：为什么 Value 不传入指针调用 Set 后会报错？

因为 golang 是值传递，传入 Value 中的值是副本，后续修改的也是副本，不会影响到原值，因此 golang 在内部做了判断

这种情况下 CanSet() 返回的是 false



### 3.2、修改变量

```go
func main() {
	a := 100
	// 获取 Value
    v := reflect.ValueOf(&a)
    v1 := v.Elem()
    fmt.Println(v.CanSet())					//false
    fmt.Println(v1.CanSet())				//true
    
	fmt.Println(v1)				//100
	
	v1.SetInt(50)	
    fmt.Println(v1)				//50
    v1.Set(reflect.ValueOf(10))	
    fmt.Println(v1)				//10
}
```



> #### 问题：为什么传入指针后还需要调用 Elem() ？

Elem() 获取的是指针指向的真实值，在 Value 结构体中同样维护了一个变量 flag，可以把它理解为是否能够进行修改

Elem() 代码如下：

```go
func (v Value) Elem() Value {
	k := v.kind()
	switch k {
	case Interface:
		// ...
	case Ptr:
		ptr := v.ptr
		if v.flag&flagIndir != 0 {
			ptr = *(*unsafe.Pointer)(ptr)
		}
		// The returned value's address is v's value.
		if ptr == nil {
			return Value{}
		}
		tt := (*ptrType)(unsafe.Pointer(v.typ))
		typ := tt.elem
        // 重新计算新的 Value 的 flag
		fl := v.flag&flagRO | flagIndir | flagAddr
		fl |= flag(typ.Kind())
		return Value{typ, ptr, fl}
	}
	panic(&ValueError{"reflect.Value.Elem", v.kind()})
}
```

我们可以看出，如果 v 是指针，那么它会重新计算指向的值的 flag

我们可以推测这样的逻辑：如果 ValueOf() 传入的是值或者指针，那么它的 flag 值在 CanSet() 中通过计算后函数返回的是 false，即它的 flag 值就表示当前 Value 不支持修改，如果传入指针的 Value v 调用 Elem() 后，它根据 v 的 flag 重新计算得到的新的 flag 表示的是新的 Value 能够进行修改

也就是说，Value 结构体中的 flag 是一个表示当前值是否能够修改的标志，它的值的设计很nb，当 Value 为值或者指针时，那么它对应的 flag 最终计算得到的是 不支持修改，如果 Value 是指针 Value 调用 Elem() 后返回的，那么它的 flag 最终计算得到的是支持修改，那么就能够进行修改



### 3.2、调用方法

```go
type People struct {
	a int
	b int64
}

func (People) Eat() {
	fmt.Println("eat")
}

func (*People) Say(i int) {
	fmt.Println("say：", i)
}

func (*People) walk() {
	fmt.Println("walk")
}

func main() {
    // 传入的是指针
    pp := &People{}
	v := reflect.ValueOf(pp)
	v.MethodByName("Eat").Call(nil)		// eat
    params := []reflect.Value{
        reflect.ValueOf(20),
    }
	v.MethodByName("Say").Call(params)	// say： 20
    
    // 传入的是值
    p := People{}
    v1 := reflect.ValueOf(p)
    v1.MethodByName("Eat").Call(nil)	// eat
	v1.MethodByName("Say").Call(params)	// panic: reflect: call of reflect.Value.Call on zero Value
    
    // 调用私有方法
    v.MethodByName("work").Call(nil)	// panic: reflect: call of reflect.Value.Call on zero Value
}
```



上面我们可以看出

- 如果 Value 传入的是指针，那么它可以反射调用 **指针接收器和值接收器** 的方法
- 如果 Value 传入的是值，那么它只能调用 **值接收器** 的方法，它无法获取到  **指针接收器** 的方法
- 反射无法获取、调用私有方法



## 4、为什么反射无法获取私有方法？



> #### Value 中 MethodByName() 调用逻辑

```go
//Value MethodByName
func (v Value) MethodByName(name string) Value {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.MethodByName", Invalid})
	}
	if v.flag&flagMethod != 0 {
		return Value{}
	}
	// 调用 value 中维护的 Type 对象的 MethodByName
	m, ok := v.typ.MethodByName(name)
	if !ok {
		return Value{}
	}
	return v.Method(m.Index)
}

// Type MethodByName
func (t *rtype) MethodByName(name string) (m Method, ok bool) {
	if t.Kind() == Interface {
		tt := (*interfaceType)(unsafe.Pointer(t))
		return tt.MethodByName(name)
	}
	ut := t.uncommon()
	if ut == nil {
		return Method{}, false
	}
    // 获取所有可导出方法列表，遍历，获取方法名匹配的，返回
	for i, p := range ut.exportedMethods() {
		if t.nameOff(p.name).name() == name {
			return t.Method(i), true
		}
	}
	return Method{}, false
}
```

可以看到 Value 的 MethodByName 实际上调用的是 Type 的 MethodByName，而在该方法内部它是通过 exportedMethods() 获取可导出方法列表再进行匹配的，因此，如果是私有方法，那么这里一定匹配不成功，因为它一开始就过滤了私有方法





## 5、IsZero()、IsNil()、IsValid() 三大方法

### 5.1、IsZero()

```go
func (v Value) IsZero() bool {
	switch v.kind() {
	case Bool:
		return !v.Bool()
	case Int, Int8, Int16, Int32, Int64:
		return v.Int() == 0
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return v.Uint() == 0
	case Float32, Float64:
		return math.Float64bits(v.Float()) == 0
	case Complex64, Complex128:
		c := v.Complex()
		return math.Float64bits(real(c)) == 0 && math.Float64bits(imag(c)) == 0
	case Array:
		for i := 0; i < v.Len(); i++ {
			if !v.Index(i).IsZero() {
				return false
			}
		}
		return true
	case Chan, Func, Interface, Map, Ptr, Slice, UnsafePointer:
		return v.IsNil()
	case String:
		return v.Len() == 0
	case Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Field(i).IsZero() {
				return false
			}
		}
		return true
	default:
		// This should never happens, but will act as a safeguard for
		// later, as a default value doesn't makes sense here.
		panic(&ValueError{"reflect.Value.IsZero", v.Kind()})
	}
}
```



该方法用来判断 Value 内部的值是否是零值

不同类型的判断方法如下：

- 基本数据类型：
  - int、int32、float32、byte、uint 等判断是否是 0
  - string 是否是 ""（len == 0）
  - bool 是否是 false
- 结构体：结构体内部所有变量是否都是零值，但凡有一个不是那么返回 false
- 数组 Array：判断内部的每个元素是否都是零值，但凡有一个不是那么返回 false
  - 实际开发中 Array 基本不用
- slice、map、chan、func、unsafe.Pointer、interface：判断是否为 nil
  - （注意，slice 的 nil 实际上是已经创建 24byte 的结构体了，只不过内部的数据指针为 nil）



> #### 例子1：测试基本数据类型

```go
func main() {
    // 1、测试基本数据类型
    var i int = 0
	v := reflect.ValueOf(i)	// 这里不能传指针，否则变成了 ptr
    fmt.Println(v.IsZero())	// true
    
    i = 1
    v = reflect.ValueOf(i)
    fmt.Println(v.IsZero())	// false
}
```



> #### 例子2：测试结构体

```go
type People struct {
	a int
	b string
}
func main() {
    p := People{}
    v2 := reflect.ValueOf(p)
    fmt.Println(v2.IsZero())	// true
    
    p = People{				// 这里不能使用 p.a = 1 直接修改上一个结构体的原因是传入 v2 中的 p 是值传递，这里修改了也不会影响到 v2 中的副本
        a:  1,
    }
    v2 = reflect.ValueOf(p)
    fmt.Println(v2.IsZero())	// false
}
```



> #### 例子3：测试 slice

```go
func main() {
	var is []int
	v3 := reflect.ValueOf(is)
	fmt.Println(v3.IsZero()) // true

	is = append(is, 1)
    v3 = reflect.ValueOf(is) // 这里不沿用上面的 v3 是因为 append() 后 is 这里绝对会发生扩容，那么原本 v3 持有的仍然是旧的 is
	fmt.Println(v3.IsZero()) // false
    
    is = []int{}
    v3 = reflect.ValueOf(is)
	fmt.Println(v3.IsZero()) // false
}
```



> #### 例子4：测试 map

```go
func main() {
	var m map[int]struct{}
	v4 := reflect.ValueOf(m)
	fmt.Println(v4.IsZero()) // true

	m = map[int]struct{}{}
	v4 = reflect.ValueOf(m)
	fmt.Println(v4.IsZero()) // false
}
```



### 5.2、IsNil()

```go
func (v Value) IsNil() bool {
	k := v.kind()
	switch k {
	case Chan, Func, Map, Ptr, UnsafePointer:
		if v.flag&flagMethod != 0 {
			return false
		}
		ptr := v.ptr
		if v.flag&flagIndir != 0 {
			ptr = *(*unsafe.Pointer)(ptr)
		}
		return ptr == nil
	case Interface, Slice:
		// Both interface and slice are nil if first word is 0.
		// Both are always bigger than a word; assume flagIndir.
		return *(*unsafe.Pointer)(v.ptr) == nil
	}
	panic(&ValueError{"reflect.Value.IsNil", v.kind()})
}
```

判断 Value 中的值是否是 nil，只能作用于能够进行 nil 判断的数据类型：slice、map、chan、func、interface、unsafe.Pointer

其他的数据类型会报错：`reflect.Value.IsNil`



### 5.3、IsValid()

```go
func (v Value) IsValid() bool {
	return v.flag != 0
}
```

判断 Value 内的值是否为 nil

- 如果不为 nil，那对应的 flag != 0，返回 true
- 如果为 nil，那对应的 flag == 0，那么返回 false

flag 可以理解为一个标识，在创建 Value 的时候会根据变量类型去计算这个 flag，如果为 nil，那么赋值为 0，其他的比如 ptr 类型的值会赋值为 flag = 22



> #### 例子1：测试基本数据类型

```go
func main() {
	i := 0
	v := reflect.ValueOf(i)
	fmt.Println(v.IsValid()) // true
}
```



> #### 例子2：测试 nil

```go
func main() {
	v2 := reflect.ValueOf(nil)
	fmt.Println(v2.IsValid())        // false，对于 go 来说，nil 只是一个标识，编译器读取到 nil 这个标识就会做出对应的操作
}
```



> #### 例子3：测试指针类型

```go
func main() {
	var ip *int = nil
	v3 := reflect.ValueOf(ip)
	fmt.Println(v3.IsValid())        // true，ip 本身是一个指针，不过它本身并没有指向任何的值而已，因此对于 v3 内部并不是 nil
    fmt.Println(v3.Elem().IsValid()) // false，ip 没有指向任何值，所以 Elem() 返回的 Value 内部为 nil
}
```

实际上 slice、map、chan 传入的效果类似 指针，Value 内部都不会是 nil，因此必定返回 true



> #### 例子4：测试结构体

```go
type People struct {
	a int
	b int64
}
func main() {
    var p People
    v4 := reflect.ValueOf(p)
    fmt.Println(v4.IsValid()) 	// true
    
    // 获取 p 中存在的字段 Value
    v5 := v4.FieldByName("a")
    fmt.Println(v5.IsValid()) 	// true
    
    // 获取 p 中不存在的字段 Value
    v5 = v4.FieldByName("f")
    fmt.Println(v5.IsValid()) 	// false
}
```

