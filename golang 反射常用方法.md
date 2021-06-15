# golang 反射常用方法

```go
var t reflect.Type
var v reflect.Value
ft = t.Filed(i)
fv = v.Field(i)	// 注意如果 v.Kind() = ptr，那么需要先调用 Elem()
// ft 是 StructFile 类型，fv 是 Value 类型

ft.Name 获取变量名
ft.Offset 获取地址偏移量
ft.Type 获取变量的类型

ft.Tag 获取字段后面跟着的标签字符串，比如 `json:"user_id" bson:"userID"` // Tag 是一个 StructTag 结构，type StructTag string

v.Type() 获取值的数据类型 // reflect.Type 没有该方法

// Map Value 方法
v.MapIndex(key Value) 获取 key 为 入参的值的 Value
v.MapKeys() []Value 获取所有的 key 的 Value 集合
v.SetMapIndex(key Value, value Value) 修改 key 对应的值
v.Len() 获取元素个数

// Slice Value 专属方法
v.Len() 获取长度
v.Cap() 获取容量
v.SetLen() 设置长度， 要求 v.Kind() == ptr 然后调用 Elem()，即我们需要传入切片指针 &sl 然后调用 Elem()
v.SetCap() 设置容量， 要求 v.Kind() == ptr 然后调用 Elem()，即我们需要传入切片指针 &sl 然后调用 Elem()



t.Kind() = v.Kind() 获取内部值的类型，比如 struct、int、func

t.Elem() 调用者 t.Kind() 必须是 ptr 类型，返回指向的值的 reflect.Type
v.Elem() 调用者 v.Kind() 必须是 ptr 类型，返回指向的值的 reflect.Value（实际上返回的 Value 内部值是同一个指针，只不过 flag 和 Kind() 发生变化）


v.Pointer() / fv.Pointer() 返回指向的值的内存地址的 uinptr 值，调用者 v.Kind() = ptr、slice、map、func、chan，
v.Addr() / fv.Addr() 返回 v 内值的指针的 Value， 类似传入 i 返回 &i。调用者 v 必须是 addressable 的，只有为可导出字段才可以修改，如果是 v.Kind() = ptr，那么需要调用 Elem()
	// 因为 Addr() 内部是直接返回 v.ptr 的封装，如果 v.Kind() == ptr，那么 v.ptr 是值的指针的指针，因此需要先调用 Elem() 解引用，得到值的指针 ptr
v.Set() / fv.Set() 调用者 v 必须是 addressable 的，即 只有为可导出字段才可以修改，如果是 v.Kind() = ptr，那么需要调用 Elem()
v.CanSet() / fv.CanSet() 用于判断当前 v 内部的值是否可修改，只有为可导出字段才可以修改，如果是 v.Kind() = ptr，那么需要调用 Elem()

reflect.Append(s Value, x...Value) Value 
	反射调用 slice 的 append()，s 必须是 slice(不能是切片指针) ，x 是 append 的数据集合，它内部会调用 MakeSlice() 创建一个新的 slice，因此 append 不是在原来的 slice 上执行的，最终将 append 完成的新的 slice Value 返回

reflect.MakeSlice(Type, len, cap) 创建一个 Type 类型的切片 Value，长度为 len，容量为 cap
reflect.MakeMap(Type) 创建一个元素类型为 Type 的 map，Type 必须是 map 类型的
```



但凡涉及到需要修改的，并且 v.Kind() == ptr 的，那么需要先调用 Elem()

