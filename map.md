# map



## map 结构

```go
// A header for a Go map.
type hmap struct {
    // map中的键值对的数量
	count     int 		
    // 状态标志
	flags     uint8		
    // 因为桶个数都是设置为 2 的幂，所以这里直接存储桶个数的对数 即 buckets = 2^B，最多容纳 6.5 * 2 ^ B 个元素，6.5为装载因子
	B         uint8  	
    // 溢出的个数
	noverflow uint16 	
    // hash seed 哈希种子
	hash0     uint32 	

    // buckets 桶数组的指针，当count == 0时，可能为nil，一个 bmap 就是一个 bucket
	buckets    unsafe.Pointer 
    // 旧的 buckets 桶数组的指针，旧的 buckets 数组大小是新的 buckets 容量的 1/2
    // 非扩容状态下, 它为 nil. 它是判断是否处于扩容状态的标识
	oldbuckets unsafe.Pointer 
    // 扩容进度，也是 buckets 的迁移进度，小于 nevacuate 的 bucket 已经迁移完成
	nevacuate  uintptr        

    // 可选参数
	extra *mapextra 	
}

// mapextra holds fields that are not present on all maps.
type mapextra struct {
    // 当 key 和 value 都不包含指针时，才可以使用 overflow
    
    // 溢出桶的指针地址
    // 存储桶数组溢出的元素，每个 bucket 最多存储 8 个 key-value，超出的元素会存储在 overflow 中
    // 如果 overflow 也溢出，那么新建一个 bucket bmap，串在 overflow 后面
    // （Java 的 HashMap 超过 8 个并且 len > 64 转换为红黑树）
	overflow    *[]*bmap
    // 旧溢出桶的指针地址
	oldoverflow *[]*bmap

    // 指向 overflow 中第一个可用（未满）的 bucket 的指针
	nextOverflow *bmap
}

// A bucket for a Go map.
type bmap struct {
	tophash [bucketCnt]uint8
}
```

