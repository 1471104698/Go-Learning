# Option



## 为什么需要 Option？

存在以下结构体：

```go
// Req
type Req struct {
	a string
	b string
	c string
	d string
    
	h time.Duration
    
	i int

	j bool
}
```

如果我们要进行初始化的话，对外提供一个 NewReq() 函数用于初始化该结构体：

```go
func NewReq(a, b, c, d string, h time.Duration, i int, j bool) *Req {
    return &Req {
        a: a,
        b: b,
        c: c,
        d: d,
        h: h,
        i: i,
        j: j,
    }
}
```

这种做法有什么不妥呢？对于如此多的参数，如果用户有时候只需要传一两个参数，但是他却需要固定去再填充其余根本不需要的字段

比如用户只需要 i 和 j 两个参数，但是他需要这么做：

```go
i := 1
j := true
req := NewReq("", "", "", "", time.Duration(0), i, j)
```

这明显是不人性化的，因此出现了这个 Option 功能，它能够让参数的传入变得可选，用户可以将想要传入的参数构造成 Option 传入即可



## Option 解决参数固定传入问题

> #### Option 设置

```go
// Req
type Req struct {
	a string
	b string
	c string
	d string

	h time.Duration
	i int

	j bool
}

// 定义一个 Option ，它是一个 func，入参为 *Req，即需要修改参数的结构体
type Option func(req *Req)

// WithA 我们构建一个 func，这个 func 入参是用户想要修改的某个参数的值，然后利用 func 的闭包性质，出参为 Option 类型的 func，
// 我们在这个 Option 内部将要修改的参数赋值给对应的结构体变量上，这样调用这个 Option 函数就可以直接修改这个变量了
// 我们需要做的就是接收用户想要修改的变量的 Option，然后执行它们，继而完成对 Req 的初始化
func WithA(a string) Option {
	return func(req *Req) {
		req.a = a
	}
}

func WithB(b string) Option {
	return func(req *Req) {
		req.b = b
	}
}

func NewReq(opts ...Option) *Req {
	req := new(Req)
	// 执行所有用户传入的 Option，以此来设置 req 的值
	for _, opt := range opts {
		opt(req)
	}
	return req
}
```



> #### 例子

```go
func main() {
	// 我们执行 Req 提供的方法，获取对应的 Option 传入
	req := NewReq(
        WithA("a"), // 想要赋值 a 参数，那么调用 a 对应的修改参数 WithA()，获取对应的 Option
        WithB("b"))
	fmt.Println(req)	// &{a b   0 0 false}
}
```



## Option 的具体应用场景

Option 不是随便用在任意一个结构体的，Option 意为选项，只是用在一些可选非必传的参数的情况下，将一些可选参数封装成一个结构体，一般命名为 `Options`，然后实现利用 Option 的设置完成参数赋值，而 `Options` 一般是作为一个字段嵌入到结构体中的

比如以下 EMS 结构体，它代表一份快递上签收人的信息：

```go
// EMS 快递信息
func EMS struct {
	name string	 //	姓名
	phone string //	手机号
	opts Options // 可选参数
}

type Option func(*Options)

func Options struct {
	age int32 // 年龄
	blood string // 血型
	height int32 // 身高
}

func WithAge(age int32) Option {
    return func(opt *Options) {
        opt.age = age
    }
}

func WithBlood(blood string) Option {
    return func(opt *Options) {
        opt.blood = blood
    }
}

func WithHeight(height int32) Option {
    return func(opt *Options) {
        opt.height = height
    }
}
```



很明显，对于一份快递信息来说 name 和 phone 是必传的，但是 Options 内部的这些参数却是非必要的了，因此可传可不传，那么此时的 EMS 初始化函数为：

```go
type NewEMS(name, phone string, opts ...Option) *EMS {
	ems := &EMS {
		name: name,
		phone: phone,
	}
	for _, opt := range opts {
		opts(ems)
	}
	return ems
}
```



用户调用为：

```go
func main() {
	name := "张三"
	phone := "1008611"
	opts := []Option {
		WithAge(11),
		WithHeight(170),
	}
	ems := NewEms(name, phone, opts...)
    fmt.Println(ems)
}
```

