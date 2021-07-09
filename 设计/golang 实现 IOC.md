# golang 实现 IOC



## 前置知识

```go
1、了解 IOC 是什么
2、golang 反射
3、如何动态创建 bean
4、如何管理 bean
5、如何标识 bean 注入
```



## 1、IOC 是什么?

IOC 直译为 **控制反转**，Java 的主流框架 Spring 的两个大特点为实现了 IOC 和 AOP，其中 IOC 的含义个人通过源码阅读，比较确定的理解是将 bean 的创建交给 Spring 去实现，由 Spring 全权完成 bean 的实例化和管理

个人认为 IOC 存在两个比较明显的作用

以下使用 Java 举例讲解，可以直接跟 Spring 对应起来



> #### IOC 的作用一：减少 bean 注入逻辑修改时代码的改动

比如下面我们存在多个类依赖了 UserDao，如果 UserDao 的实例化逻辑发生改变，那么对于依赖它的 A、B 等类是不需要修任何代码的

```java
class A{
    @Autowire
	UserDao userdao;
}
class B{
    @Autowire
	UserDao userdao;
}
class C{
    @Autowire
	UserDao userdao;
}
class D{
    @Autowire
	UserDao userdao;
}
```



> #### IOC 的作用二：解决 bean 的嵌套依赖注入

这个 bean 内部可能依赖了多个 bean，而依赖的 bean 可能又依赖了其他的 bean，bean 的依赖可能是嵌套的

如果由用户来创建的话，那么需要由用户手动去解决这个嵌套关系：

```java
class Main{
	A a = new A(new B(new C(new D())));
}
```

而一旦 D 中需要再引用一个 E，或者 不再需要 D，这样就需要去改动代码了：

```java
//D 引用一个 E
class Main{
	A a = new A(new B(new C(new D(new E()))));
}
//不需要 D
class Main{
    A a = new A(new B(new C()));
}
```

而可能这个 A 类在很多地方都需要用到，这样就需要多处改动代码。

当然，这样的话，我们可能就会想到使用 **工厂模式**，将创建 A 的逻辑给封装起来，这样的话，所有地方调用都只需要调用这个方法获取一个 A 对象，而我们修改也只需要修改这个方法即可

```java
class AFactory{
	public static A createA(){
        return new A(new B(new C(new D())));
    }
}
```

同理，Spring 的 **IOC 内部创建 bean 也是使用了 工厂模式 中的简单工厂模式**，只对外提供一个 getBean()，bean 的创建在 getBean() 中进行封装，通过方法入参来标识需要创建的对象

大致逻辑如下：

（上面的是构造传参，这里就不说构造传参了，说变量注入，**原理都是通过反射**）

在生成 bean 的时候会获取所有的成员变量，判断是否需要注入，如果需要，那么就创建注入对象进行注入

以下是 IOC 的简单伪代码：

```java
class BeanFactory{
    // createBean 根据 className 实例化对应的 bean
    public<T> T	createBean(String className){
        try{
            // 根据 className 获取对应的 class 对象
            Class<?> clazz = Class.forName(className);
            // 初始化一个实例
            Object bean = clazz.newInstance();
            //处理属性注入
            handleBean(clazz, bean);
            return (T)bean;
        }catch(Exception e){
            throw new RuntimeException(e);
        }
    }
    // handleBean 根据 class 对象实例化一个 bean，同时进行属性注入
    private void handleBean(Class<?> clazz, Object o) throws IllegalAccessException {
        //获取 clazz 所有的成员变量
        Field[] fs = clazz.getDeclaredFields();
        // 扫描所有的变量
        for(Field field : fs){
            //获取当前变量 field 的所有的注解
            Annotation[] annotations = field.getAnnotations();
            //判断当前 field 是否存在 "Autowire" 注解
            for(Annotation annotation : annotations){
                //如果存在 "Autowire" 注解
                if("Autowire".equals(annotation)){
                    // 递归创建
                    Object bean = createBean(field.getClass().getName());
                    //将 bean 注入进去
                    field.set(o, bean);
                }
            }
        }
    }
}
```

IOC 会自动在 C 中注入 D，在 B 中注入 C，在 A 中注入 B，然后将 A 返回，无需我们自己手动添加这层关系

（真正的 Spring IOC 处理很复杂，它每个 bean 的构造都是通过八种类型的后置处理器来实现的，每种后置处理器位于不同的位置，分别做不同逻辑的处理，这八种后置处理器能够处理所有的 bean，在处理过程中还同时解决了 循环依赖 + AOP，具体可以看整理的笔记：[spring 笔记](https://github.com/1471104698/leetcode/tree/master/A/Spring)）



## 2、goalng 反射

golang 反射这里整理了一篇笔记：[golang 反射](https://github.com/1471104698/Go-Learning/blob/master/Golang%20%E5%8F%8D%E5%B0%84.md)

golang 反射跟 Java 反射有比较大的差别

goalng 反射有两种主要的类型 reflect.Type 和 reflect.Value，对应的方法为 reflect.TypeOf(val) 和 reflect.ValueOf(val)

1. reflect.Type 用来获取类型信息（数据类型、持有的方法、持有的变量等），比如它只能获取 val 的数据类型，以及 val 存在多少个变量、实现了多少个方法、以及对应的变量名、方法名等，但是不能获取它内部变量的真实值，无法进行重新赋值，无法获取实现的方法本身，即无法进行调用

2. reflect.Value 用来获取真实数据（值、持有的方法本身、持有的变量本身），它内部维护了一个 reflect.Type，并且它内部有一个 ptr 指针，指向 val，即 reflect.Value 拥有 reflect.Type 的所有方法，并且它本身能够获取所有变量的值并重新赋值，获取所有方法本身并调用它们（**误**）

这里有个问题，既然 reflect.Value 内部维护了一个 reflect.Type，那么实际上不是变相说明 reflect.Type 对外提供的接口完全可以利用 reflect.Value 来完成么，那么这个 reflect.Type 作用是什么？

实际上虽然 reflect.Type 和 reflect.Value 都有 FieldByName() 这类方法，但是它们的返回值是不一样的

1. reflect.Type 返回的是 StructField 结构体，内部记录了当前 field 的所有类型信息，比如 Offset（相对结构体的起始地址偏移量）、Tag（类似 Java 注解）、Index（在结构体所有变量中的索引位置）
2. reflect.Value 返回的是 Value，相当于是覆盖了内部维护的 reflect.Type 的方法，返回的是 field 对应的数据信息，内部是指向 field 数据的 ptr



同时 golang 反射与 Java 反射一个很大的差异点是 **golang 反射无法获取到私有方法**，在 MethodByName() 中，它是调用 exportedMethods() 获取所有的可导出方法，然后利用 name 来进行字符串匹配的，如果匹配成功才会返回，对于私有方法不会存在于 exportedMethods() 中，所以不会匹配成功，返回的是一个空的 Method，所以导致 golang 无法获取到私有方法



另一个 golang 反射的重点：为什么对于 reflect.ValueOf(val) 传入的 val 是 ptr 类型时，在调用 set 赋值的时候需要调用 Elem()？

上面的笔记讲得比较详细，这里粗略讲下（这里网上没找到对应的讲解，因此这里是自己根据源码粗略的解读的，**所以难免会有错误的地方**）

我们先看下 reflect.ValueOf() 源码：

```go
func ValueOf(i interface{}) Value {
	if i == nil {
		return Value{}
	}

	// TODO: Maybe allow contents of a Value to live on the stack.
	// For now we make the contents always escape to the heap. It
	// makes life easier in a few places (see chanrecv/mapassign
	// comment below).
	escapes(i)

	return unpackEface(i)
}

// unpackEface converts the empty interface i to a Value.
func unpackEface(i interface{}) Value {
    // 这里将获取 i 的指针，转换为 unsafe.Pointer，然后强转为 *emptyInterface
	e := (*emptyInterface)(unsafe.Pointer(&i))
	// NOTE: don't read e.word until we know whether it is really a pointer or not.
	t := e.typ
	if t == nil {
		return Value{}
	}
	f := flag(t.Kind())
	if ifaceIndir(t) {
		f |= flagIndir
	}
    // 最终这里将 e.word 赋值给 Value 的 ptr，e.word 也是一个 unsafe.Pointer 类型，实际上就是上面强转的 &i
	return Value{t, e.word, f}
}
```

我们可以看到，对于传入的 i 值，它是先获取 &i 再最终存储到 Value 中的，也就是说，对于 i  为非 ptr 来说，存储到 Value 中的数据的指针，而对于 i 为 ptr 来说，那么 Value 存储的就是数据的指针的指针了。

然后我们再看看 Elem() 方法：

```go
// Elem returns the value that the interface v contains
// or that the pointer v points to.
// It panics if v's Kind is not Interface or Ptr.
// It returns the zero Value if v is nil.
func (v Value) Elem() Value {
	k := v.kind()
	switch k {
	case Interface:
		// xxxx
	case Ptr:
        // 1、ptr 赋值为 v.ptr
		ptr := v.ptr
		if v.flag&flagIndir != 0 {
            // 2、这里可以简单理解为 ptr 赋值为 *v.ptr，可以理解为解引用，即如果是 v.ptr 是指针的指针，那么这里就是变成 指针，去除第二层指针
			ptr = *(*unsafe.Pointer)(ptr)
		}
		// The returned value's address is v's value.
		if ptr == nil {
			return Value{}
		}
		tt := (*ptrType)(unsafe.Pointer(v.typ))
		typ := tt.elem
		fl := v.flag&flagRO | flagIndir | flagAddr
		fl |= flag(typ.Kind())
        // 3、将解引用后的 ptr 存储到 Value 中
		return Value{typ, ptr, fl}
	}
	panic(&ValueError{"reflect.Value.Elem", v.kind()})
}
```

我们可以看到，如果 v.ptr 是指针的指针，那么它会进行解引用，将第二层指针去除，这样 ptr 就变成了数据的指针了，然后将 ptr 存储到 Value 返回

因为指针的指针不能直接操作数据，所以这里需要进行解引用，对于非指针的指针的数据，该方法没有太大的作用，该方法主要作用就是处理指针的指针





## 3、如何动态创建 bean

Java 对每个类都维护了一个 class 对象，在程序启动类加载的时候就创建好对应类的 class 对象了，class 对象是反射的入口，通过 ClassName 可以获取对应的 class 对象，再通过 class 对象动态创建实例。

golang 本身并没有跟 Java 一样维护 class 对象这种东西，因此实际上 golang 本身并不支持实例的动态创建，因此我们需要转换为另一种创建实例的方式来接近实例的动态创建：

```go
type People struct {
	age int32
	name string
}
func main() {
	t := reflect.TypeOf((*People)(nil)).Elem()
	v := reflect.New(t).Elem().Interface()
	fmt.Println(v)
}
```

1、通过 `reflect.TypeOf()`  获取对应的 reflect.Type

2、通过 `reflect.New()` + reflect.Type 创建对应的实例

由于 reflect.TypeOf() 需要传入一个 val，所以这里传入` (*People)(nil)`，这样可以避免实例化又能够让 `reflect.TypeOf()` 获取到对应的 reflect.Type，因为它并不需要对应的真实数据，所以传入的 val 只要存在类型信息即可

（这里不能使用 `People(nil)`，因为结构体不能使用 nil，编译不会报错，但是运行会报错：`cannot convert nil to type People`）



## 4、如何管理 bean

bean 的类型分为两种：单例 和 原型

1. 单例 bean 注册一次需要保存下来，后续多次注册无效，获取时返回第一次注册的 bean

2. 原型 bean 是临时 bean，注册不保存，每次获取都返回一个新的 bean

理论上我们只需要使用一个容器来存储单例 bean 实例，但是实际上如果我们只是保存单例 bean 实例，那么这个 bean 管理对原型 bean 来说没有什么太大的意义，或者说对原型 bean 没起任何作用。

这里的 bean 管理是要我们保存 bean 的类型信息，在需要的时候根据 bean 的类型信息去创建一个实例返回，如果我们单单保存单例 bean 实例，那么我们需要一个原型 bean 的时候，我们并不知道这个原型 bean 是什么东西，也就不知道如何去创建了，因为外部获取 bean 肯定是通过 bean name 来获取的。

因此在 IOC 容器中我们需要使用两个 bean 容器来存储这两种类型的 bean 的类型信息，由于需要根据 bean name 来查找对应的 bean，因此这里使用 map，**[key：bean name，value ：bean 类型信息 reflect.Type]**

这就是说我们注册 bean 的时候是将 bean name 和 reflect.Type 绑定注册到 IOC 容器中，获取 bean 的时候通过 bean name 获取



## 5、如何标识 bean 注入

在 Java Spring 中，通过 @Autowire 和 @Resource 注解来标识注入该 bean，这里我们也可以使用相同的方式来标识 bean 注入，golang 等同于 Java 注解的就是 tag：

```go
type People struct {
    age int32 	`json:"age", bson:"age"`
    name string `json:"name", bson:"age"`
}
```

上面每个变量右边的 ``json:"age", bson:"age"` 就是一个 tag，我们可以通过 `reflect.TypeOf().Field().Tag` 来获取变量 field 的 Tag，然后通过判断 Tag  来判断 bean 的注入情况



> #### 设计一

设置注入的注解为 `di`，标识注入的 bean 的类型，通过 `beanName` 选择注入特定的 bean：

```go
type A struct {
	b B `di:"singleton"`
}

type B struct {
	age int32
}
```

判断是否需要注入：

```go
func main() {
    t := reflect.TypeOf(A{})
    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        // 获取 di 标签对应的 value
        diTag := field.Tag.Get("di")
        // 不存在，那么跳过
        if diTag == "" {
            continue
		}
        // 注入逻辑
        // ....
    }
}
```



> #### 设计二

这种设计跟 Spring IOC 一样，所有的 bean 都在 Spring 初始化的时候加载完毕，然后 bean 注入只能选择注入哪个 beanName 的 bean，不能选择注入的 bean 类型，`di` 注解值就是指定注入的 beanName，如果没有指定值，那么从已经加载的 bean 中找到相同类型的 bean 选择一个注入

这里作用同 @Autowired + @Qualifier

```go
type A struct {
	B *B `di:""`
}

type B struct {
	name string
	age  int
	C    *C `di:"c"`
	A    *A `di:"a"`
}

type C struct {
	i    int
	b    bool
	name string
	A    *A `di:"a"`
}
```



## 6、IOC 结构设计（非最终结构）

> #### Bean 容器
>
> 两种 bean 容器，一个处理单例 bean，一个处理原型 bean

```go
// Container bean 容器接口
type Container interface {
	// Get 根据 beanName 获取 bean
	Get(beanName string) interface{}
}

// 单例 bean 容器
// SingletonContainer 单例 bean 容器
type SingletonContainer struct {
	// 维护 beanFactory
	BeanFactory
}
func (sc *SingletonContainer) Get(beanName string) interface{} {}

// PrototypeContainer 原型 bean 容器
type PrototypeContainer struct {
	// 维护 beanFactory
	BeanFactory
}
func (pc *PrototypeContainer) Get(beanName string) interface{} {}
```



> #### BeanFactory
>
> 生产 bean 的工厂

```go
type BeanType string
var (
	Singleton = 1
    Prototype = 2
)

// BeanFactory bean 工厂接口
type BeanFactory interface {
	// Register 注册一个 bean
	Register(class *Class) error
	// RegisterBeanProcessor 注册 bean 处理器
	RegisterBeanProcessor(class *Class) error
	// GetBean 根据 beanName 获取 bean
	GetBean(beanName string) interface{}
	// getSingleton 获取单例 bean（这里以后学习 Spring 建立三级缓存解决循环依赖）
	getSingleton(beanName string, allowEarlyReference bool) interface{}
	// createBean 创建 bean 实例
	createBean(beanName string, beanType BeanType) interface{}
	// addSingleton 添加单例 bean
	addSingleton(beanName string, i interface{})
	// isAllowEarlyReference 是否允许循环依赖
	isAllowEarlyReference() bool
}

// BeanBeanFactory bean 工厂实现
type BeanBeanFactory struct {
	// 维护单例 bean 容器
	sc Container
	// 维护原型 bean 容器
	pc Container
	// 维护所有注册 bean 的类型
	btMap map[string]BeanType
	// 维护所有注册 bean 的类型信息
	tMap map[string]reflect.Type
	// 维护所有的单例 bean，一级缓存
	singletonMap map[string]interface{}
	// 维护早期暴露对象，用于解决循环依赖，二级缓存
	earlyMap map[string]interface{}
	// 工厂 map，三级缓存，用于 AOP bean
	factoryMap map[string]func() interface{}
	// 当前正在创建的 bean 列表
	creatingMap map[string]interface{}
	// bean 处理器集合
	beanProcessors []BeanProcessor
	// 可选参数
	opts *Options
}

// Option
type Option func(*Options)

// Options beanFactory 可选参数
type Options struct {
	// 是否允许暴露早期对象
	allowEarlyReference bool
}
```



> #### IOC
>
> 对外接口

```go
// Class 存储要注册的 bean 的信息
type Class struct {
	beanName string
	i        interface{}
	beanType BeanType
}

type IOC struct {
    // 维护一个 bean 工厂
    beanFactory BeanFactory
}

func (ioc *IOC) Register(i interface{}) {}
func (ioc *IOC) GetBean(beanName string) interface{} {}
func (ioc *IOC) GetBeanFactory() BeanFactory 
```





## 7、bean 注册/获取过程

bean 注册代码：

```go
// Register 注册一个 bean 到 beanFactory 中
func (bc *BeanBeanFactory) Register(class *Class) error {
	beanName := class.beanName
	beanType := class.beanType
	i := class.i
	if !isSingleton(beanType) && !isPrototype(beanType) {
		return fmt.Errorf("beanType: %v 不符合要求\n", beanType)
	}
	// 判断 beanName 是否已经注册过了，因为 beanName 是唯一标识，所以不能重复
	if bc.isRegistered(beanName) {
		return fmt.Errorf("beanName was registered by other bean")
	}
	var t reflect.Type
	t, ok := i.(reflect.Type)
	if !ok {
		// 这里不调用 Elem()，因为可能注册的就是一个指针类型，因此这里不做指针处理
		t = reflect.TypeOf(i)
	}
	bc.btMap[beanName] = beanType
	bc.tMap[beanName] = t
	return nil
}
```

bean 注册大致逻辑：

1、判断注册的 beanType 是否是单例或者原型，如果都不是那么是非法类型，返回 err

2、由于 beanName  是 bean 的唯一标识，所以需要根据 beanName 判断该 bean 是否已经注册过了，如果已经注册过了，那么返回 err

3、记录 beanName 对应的 beanType 和 beanName 对应的 reflect.Type



bean 获取逻辑调用链：GetBean() -> createBean() -> doCreateBean()

GetBean() 代码：

```go
// GetBean 根据 beanName 获取 bean 实例
func (bc *BeanBeanFactory) GetBean(beanName string) interface{} {
	// 处理 createBean 抛出的 panic
	//defer func() {
	//	if err := recover(); err != nil {
	//		fmt.Println(err)
	//	}
	//}()
	// 获取 bean 类型
	beanType := bc.getBeanType(beanName)
	// bean 不存在
	if beanType == Invalid {
		return nil
	}
	var bean interface{}
	if isSingleton(beanType) {
		bean = bc.sc.Get(beanName)
	} else {
		bean = bc.pc.Get(beanName)
	}
	return bean
}
```

GetBean() 大致逻辑如下：

1、根据 beanName 获取对应的 beanType

2、根据 beanType 调用对应的 container 的 Get() 逻辑获取 bean



在 container 中如果需要的话会尝试从缓存中获取 bean，如果没有的话那么调用 createBean() 创建 bean

createBean() 代码如下：

```go
// createBean 创建 bean 实例
func (bc *BeanBeanFactory) createBean(beanName string, beanType BeanType) interface{} {
	// bean 创建的前置处理
	bc.createBefore(beanName, beanType)
	// bean 创建完毕的后置处理
	defer bc.createAfter(beanName, beanType)

	// 获取 bean 类型信息
	t, exist := bc.tMap[beanName]
	if !exist {
		return nil
	}
	// 创建 bean 前看该 bean 是否存在特殊创建逻辑
	bean := bc.resolveBeforeInstantiation(beanName, t)
	if bean != nil {
		return bean
	}
	// 创建 bean
	return bc.doCreateBean(beanName, t)
}
```

createBean() 大致逻辑如下：

1、首先进行前置处理，这里是将 bean 标识为正在创建，避免多个 goroutine 多次创建同一个 bean，导致单例 bean 存在多个

2、获取 beanName 对应的 reflect.Type，如果不存在表示 beanName 还没有注册，那么无法创建，直接返回

3、在创建 bean 前先调用 bean 处理器，如果返回了 bean，那么不再执行 doCreateBean() 逻辑，这里是留给用户自己构造特定 bean 的创建逻辑

4、调用 doCreateBean() 创建 bean



doCreateBean() 代码如下：

```go
// doCreateBean 真正的创建 bean 实例逻辑
func (bc *BeanBeanFactory) doCreateBean(beanName string, tPtr reflect.Type) interface{} {
	// 非 ptr type
	var t reflect.Type
	if tPtr.Kind() == reflect.Ptr {
		t = tPtr.Elem()
	} else {
		t = tPtr
	}
	// 判断当前 beanName 对应的 reflect.Type 是否能够作为 bean
	if !isBean(t) {
		return nil
	}
	// 创建实例
	beanPtr := reflect.New(t)
	// 非 ptr bean value
	bean := beanPtr.Elem()

	// 判断是否允许暴露早期对象
	if bc.opts.allowEarlyReference {
		if t == tPtr {
			// 非 ptr bean
			bc.addSingletonFactory(beanName, bean.Interface(), t)
		} else {
			// ptr bean
			bc.addSingletonFactory(beanName, beanPtr.Interface(), t)
		}
	}
	// 属性注入
	bc.populateBean(bean, t)

	// 初始化 bean，这里会执行 AOP 处理
	// 注意这里需要传入 ptr bean，为了跟下面的 getSingleton 对齐
	bean2 := bc.initializeBean(beanName, beanPtr.Interface(), t)

	// 上面存在两种 bean，一种是原始的 bean1，一种是 initializeBean 初始化返回的 bean2
	// 创建 A bean 的时候有以下几种情况：
	// 	1、创建 A 的时候 A 作为早期对象暴露了，那么如果 A 依赖了 B，B 依赖了 A，那么 A 会被拿出放到 earlyMap 中
	//	  如果 A 需要 AOP 的话，那么 AOP 对象就在 earlyMap 中，那么 initializeBean 返回的就是原始 bean
	// 	2、创建 A 的时候 A 不作为早期对象暴露，或者没有构成循环依赖，那么 initializeBean 中返回的可能是 AOP bean
	// 综上，我们实际上需要再获取 earlyMap 中的 bean3，bean2 和 bean3 之间具有以下关系：
	// 	1、如果 A 没有暴露早期对象或者没有循环依赖，那么 bean2 就是最终需要返回的 bean
	// 	2、如果 A 存在循环依赖，那么 bean3 就是最终需要返回的 bean
	var resBean interface{}
	// 允许循环依赖
	if bc.isAllowEarlyReference() {
		// 判断是否出现了循环依赖
		// 这里的 resBean 就是上面讲的 bean3
		resBean = bc.getSingleton(beanName, false)
		// 为空，那么没有出现循环依赖，那么最终 bean 为 bean2
		if resBean == nil {
			resBean = bean2
		}
	} else {
		// 不允许循环依赖，那么最终 bean2 为 bean2
		resBean = bean2
	}
	// 返回非 ptr bean
	if t == tPtr {
		// resBean 是 ptr，所以这里借助 reflect.Value 返回非 ptr
		return reflect.ValueOf(resBean).Elem().Interface()
	}
	// 返回 ptr bean
	return resBean
}
```

doCreateBean() 大致逻辑如下：

1、划分两种类型的 type：tPtr 和 t，这里的 tPtr 并非一定是 reflect.Ptr 类型，只是为了进行区分，t 用于后续 reflect.New() 创建初始 bean 以及后续 bean 返回

2、根据 t 判断是否能够作为一个 bean，如果不能那么直接返回

3、调用 reflect.New() 创建一个初始 bean，然后调用 Elem() 获取 非 ptr bean

4、判断是否支持循环依赖，如果支持的话那么将 bean 封装到第三级缓存的工厂方法中

5、调用 populateBean() 进行属性注入

6、调用 initializeBean() 进行 bean 初始化，这里会进行 AOP 处理

7、根据循环依赖情况获取最终返回的 bean，具体分析看代码注释



## 8、属性注入

> #### 设计一

processPropertyValues() 属性注入代码如下：

```go
// processPropertyValues 属性注入
func (bp *PopulateBeanProcessor) processPropertyValues(wrapBean reflect.Value, t reflect.Type) {
	// 扫描所有的 field
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// field 的 reflect.Type 类型信息
		ftPtr := field.Type
		// field 的 非 ptr type
		var ft reflect.Type
		if ftPtr.Kind() == reflect.Ptr {
			ft = ftPtr.Elem()
		} else {
			// 不允许非 ptr 结构体注入
			if !bp.bc.isAllowPopulateStructBean() {
				continue
			}
			ft = ftPtr
		}
		// 非 wrapBean，那么直接跳过
		if !isBean(ft) {
			continue
		}
		// 获取注入类型
		fieldBeanType := getFieldBeanType(field)
		// 不存在 di 注解，那么当前 field 不需要注入，那么跳过2
		if fieldBeanType == Invalid {
			continue
		}
		// 获取 field 对应注解的 beanName
		fieldBeanName := getFieldBeanName(bp.bc, field, ft)
		// 判断是否需要注册到 beanFactory 中
		if !bp.bc.isRegistered(fieldBeanName) {
			// 注册到 beanFactory 中
			_ = bp.bc.Register(NewClass(fieldBeanName, ftPtr, fieldBeanType))
		}
		// 调用 GetBean() 获取 field wrapBean，走 container 的逻辑
		fieldBean := bp.bc.GetBean(fieldBeanName)
		// 获取不到 wrapBean，那么跳过
		if fieldBean == nil {
			continue
		}
		// 将 wrapBean 封装为 reflect.Value，用于 set()
		fieldBeanValue := reflect.ValueOf(fieldBean)
		// 将 field wrapBean 赋值给 wrapBean
		if ft == ftPtr {
			// field 非 ptr，那么直接设置即可
			wrapBean.Field(i).Set(fieldBeanValue)
		} else {
			// field ptr，那么需要 fieldBean 是 ptr wrapBean，这里需要先进行 Elem()，然后 Addr() 返回地址，赋值给 field
			wrapBean.Field(i).Set(fieldBeanValue.Elem().Addr())
		}
	}
}
```

processPropertyValues() 大致逻辑如下：
1、扫描所有的 field

2、划分当前 field 的  tPtr 和 t，同时如果 field 是非 ptr 结构体，那么判断是否允许非 ptr 结构体注入

3、判断当前 field 是否能够作为一个 bean，如果不能那么跳过

4、**获取当前 field 的 beanType，判断是否非法，如果非法那么跳过**

5、**获取当前 field t 对应的 beanName，判断当前 field 对应的 beanName 是否已经注册，如果没有注册那么表示不存在那么将当前 tPtr 进行注册**

6、调用 GetBean() 走 container 的获取逻辑，自动根据 beanType 进行不同逻辑的获取

7、将返回的 bean 封装为 reflect.Value，调用 Set() 进行注入

reflect.Value 对于传入的值维护的都是指针，因此这里 Set() 是能够影响到原值的



> #### 设计二

processPropertyValues() 属性注入代码如下：

```go
// processPropertyValues 属性注入
func (bp *PopulateBeanProcessor) processPropertyValues(wrapBean reflect.Value, t reflect.Type) {
	// 扫描所有的 field
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// field 的 reflect.Type 类型信息
		ftPtr := field.Type
		// field 的 非 ptr type
		var ft reflect.Type
		if ftPtr.Kind() == reflect.Ptr {
			ft = ftPtr.Elem()
		} else {
			// 不允许非 ptr 结构体注入，那么直接跳过该 field
			if !bp.bc.isAllowPopulateStructBean() {
				continue
			}
			ft = ftPtr
		}
		// 非 bean，那么直接跳过
		if !isBean(ft) {
			continue
		}
		// 获取 field 注入的 bean 的 beanName
		fieldBeanName := bp.getFieldBeanName(field, ftPtr, ft)
		// beanName 不存在，报错
		if fieldBeanName == "" {
			panic(fmt.Errorf("field bean %v is not exist", field.Name))
		}
		// 调用 GetBean() 获取 field bean，走 container 的逻辑
		fieldBean := bp.bc.GetBean(fieldBeanName)
		// 将 bean 封装为 reflect.Value，用于 set()
		fieldBeanValue := reflect.ValueOf(fieldBean)
		// 将 field bean 赋值给 bean
		if ft == ftPtr {
			// field 非 ptr，那么直接设置即可
			wrapBean.Field(i).Set(fieldBeanValue)
		} else {
			// field ptr，那么需要 fieldBean 是 ptr bean，这里需要先进行 Elem()，然后 Addr() 返回地址，赋值给 field
			wrapBean.Field(i).Set(fieldBeanValue.Elem().Addr())
		}
	}
}
```

processPropertyValues() 大致逻辑如下：
1、扫描所有的 field

2、划分当前 field 的  tPtr 和 t，同时如果 field 是非 ptr 结构体，那么判断是否允许非 ptr 结构体注入

3、判断当前 field 是否能够作为一个 bean，如果不能那么跳过

4、**获取 beanName，这里是根据 DI 注解进行获取，如果存在 DI 注解但是没有具体的值，那么从已经注册的 bean 中获取相同类型的 bean，再获取对应的 beanName**

5、**如果 beanName == ""，表示该 field 没有注册，那么 panic，注入失败**

6、调用 GetBean() 走 container 的获取逻辑，自动根据 beanType 进行不同逻辑的获取

7、将返回的 bean 封装为 reflect.Value，调用 Set() 进行注入

reflect.Value 对于传入的值维护的都是指针，因此这里 Set() 是能够影响到原值的





## 9、循环依赖

存在以下结构体：

```go
type A struct {
	B *B `di:"s" beanName:"bbbb"`
}

type B struct {
	name string
	age  int
	C    *C `di:"s"`
}

type C struct {
	i    int
	b    bool
	name string
	A    *A `beanName:"a" di:"s"`
}
```

它们之间存在循环依赖：A 依赖 B，B 依赖 C，C 依赖 A，并且它们都是单例的，这样的话普通的属性注入就会存在以下的问题：

创建 A 的过程中发现依赖了 B，那么就会去创建 B，创建 B 的过程中发现依赖了 C，那么就会去创建 C，创建 C 的过程中发现依赖了 A，那么又会再去创建 A，而 A 是单例的，并且 A 已经在创建了，那么此时就不能再去创建 A，这时候可以选择直接返回或者 panic



循环依赖的问题 Spring 有一个很好的解决方法：使用三级缓存

beanFactory 内部维护三级缓存：

```go
// 维护所有的单例 bean，一级缓存
singletonMap map[string]interface{}
// 维护早期暴露对象，用于解决循环依赖，二级缓存
earlyMap map[string]interface{}
// 工厂 map，三级缓存，用于 AOP bean
factoryMap map[string]func() interface{}
```

getSingleton() 代码如下：

```go
// getSingleton 获取单例 bean（建立三级缓存解决循环依赖）
func (bc *BeanBeanFactory) getSingleton(beanName string, allowEarlyReference bool) interface{} {
	// 从单例池中获取
	bean := bc.singletonMap[beanName]
	// 单例池不存在 bean 并且允许循环依赖
	if bean == nil {
		// 从早期暴露对象池中获取 bean
		bean = bc.earlyMap[beanName]
		if bean == nil && allowEarlyReference {
			// 从三级缓存中获取
			singletonFactory := bc.factoryMap[beanName]
			if singletonFactory != nil {
				bean = singletonFactory()
				// 将 bean 放到早期对象池中，下次获取直接从早期对象池中获取
				bc.earlyMap[beanName] = bean
			}
		}
	}
	return bean
}
```

当 A 创建的时候，在属性注入之前，将 A 放入到第三级缓存中，属性注入的时候发现 A 依赖了 B，那么创建 B，在属性注入之前，将 B 放入到第三级缓存中，属性注入的时候发现 B 依赖了 C，那么创建 C，同理，然后属性注入的时候发现 C 依赖了 A，那么尝试创建 A，在创建 A 之前调用 getSingleton() 尝试从三级缓存中获取 A，这时候在 singletonMap 和 earlyMap 都无法获取到 A，然后会在 factoryMap 中获取到 A 对应的工厂方法，调用该工厂方法，获取返回的 bean，然后将该 bean 存储到 earlyMap 中，返回，此时将半成品的 A 注入到 C 中，然后再将 C 返回，将 C 注入到 B 中，然后将 B 返回，将 B 注入到 A 中， A 继续完成初始化，等到初始化完成后，将 A 返回，完成 A bean 的创建。

```go
这里对该过程的几个特殊点进行解释：

1、C 虽然注入的是半成品的 A，但是实际上它持有的是对 A 的引用，当 A 初始化完成后，C 维护的 A 也会变成一个成品

2、这里的 factoryMap 是为了处理 A 需要 AOP 的情况，在工厂方法内部会判断 A 是否需要进行 AOP，如果需要的话那么进行 AOP 处理

3、将工厂方法返回的 bean 放入到 earlyMap 的原因是避免多余的工厂方法调用，下次获取 A 半成品直接从 earlyMap 中获取即可，不需要再走工厂方法
```



## 10、AOP

目前 golang 没有找到一个完善的 AOP 实现方式，因此这里功能暂且搁置，后续找到了那么再进行补充



## 11、遇到的问题以及解决

> #### 问题一

循环依赖中 struct 注入的问题

有以下结构体：

```go
type A struct {
	B *B `di:""`
}

type B struct {
	name string
	age  int
	C    *C `di:"c"`
	A    *A `di:"a"`
}

type C struct {
	i    int
	b    bool
	name string
	A    A `di:"a"`
}
```

上面的结构体中存在循环依赖：A -> B -> C -> A，而 C 中依赖的 A 是 struct bean

```go
func main() {
	opts := []gioc.Option{
		gioc.WithAllowEarlyReference(true),
		gioc.WithAllowPopulateStructBean(true),
	}
	ioc := gioc.NewIOC(opts...)
	// 这里在 Spring 中应该是由 Spring 扫描类路径然后获取 @Component 或者 @Import 注解的类的信息然后再注册的，我这里省去了扫描的过程，直接构建注册
	classA := gioc.NewClass("a", (*A)(nil), gioc.Singleton)
	classB := gioc.NewClass("bbbb", (*B)(nil), gioc.Singleton)
	classC := gioc.NewClass("c", (*C)(nil), gioc.Singleton)
	_ = ioc.Register(classA)
	_ = ioc.Register(classB)
	_ = ioc.Register(classC)
	bean := ioc.GetBean("a").(*A)
	fmt.Println(bean)
}
```

我们获取 A bean，它在创建的过程中会去创建 B，然后会再去创建 C，C 会再去创建 A，第二次调用创建 A 的过程中，会从三级缓存中获取到 A，此时的 A 是一个半成品，由于 C 要求的是一个 struct bean，因此 C 注入的是 struct bean，它是正在创建的 A 的副本，因此当 A 创建完成后不会影响到副本，C 中注入的 A 也仍然是一个半成品，最终返回的 bean 中 C 的内容如下：

![image.png](https://pic.leetcode-cn.com/1625847196-UjQLHS-image.png)

> #### 解决

鉴于注入的 struct bean 都是一个副本，因此这里判断如果注入的是 struct bean，那么直接创建一个新的 bean，不使用原有的 bean

```go
// processPropertyValues 属性注入
func (bp *PopulateBeanProcessor) processPropertyValues(wrapBean reflect.Value, t reflect.Type) {
	// 扫描所有的 field
	for i := 0; i < t.NumField(); i++ {
		// ...
        
		// 调用 GetBean() 获取 field bean，走 container 的逻辑
		var fieldBean interface{}
		// 如果是 struct bean，那么不使用旧的 bean，直接获取一个新的 bean
		if isStructBean(ftPtr, ft) {
			fieldBean = bp.bc.GetNewBean(fieldBeanName)
		} else {
			fieldBean = bp.bc.GetBean(fieldBeanName)
		}
		// ...
	}
}

// createBean 创建 bean 实例
func (bc *BeanBeanFactory) createBean(beanName string, beanType BeanType, new bool) interface{} {
    // 判断是否是需要创建新的 bean，如果是的话那么不进行前置检查
	if !new {
		// bean 创建的前置处理
		bc.createBefore(beanName, beanType)
		// bean 创建完毕的后置处理
		defer bc.createAfter(beanName, beanType)
	}
	// ...
}
```



## 12、后记

IOC 的实现是跟在 goroutine pool 之后，所一些接口之类的设计会跟 goroutine pool 有点相像。

这次 IOC 的实现是根据之前阅读过的 Spring IOC 源码来实现的简化版，实现过程也比较顺利，没有遇到过太多的问题，遇到的一些问题也能在比较短的时间内解决，大部分是反射方面的问题，golang 反射的使用比 Java 难很多，坑点也比 Java 的多很多，经过这次设计让自己对 golang 反射也更加的熟练。

IOC 的结构也改了好几次，不过这里没说明出修改的设计思路，具体可以看 github 的提交 commit。



当前设计的 IOC 存在的不足：**没有实现 AOP 和 解决多 groutine 的并发问题**

