package main

type Cache struct {
	head     *Node
	end      *Node
	mp       map[int]*Node
	capacity int
}

type Node struct {
	key int
	val int
	pre *Node
	nxt *Node
}

// New 创建一个最多容纳 capacity 个键的缓存。
// capacity 保证大于等于 0；负数容量不在本题输入范围内。
func New(capacity int) *Cache {
	cache := &Cache{
		head:     &Node{},
		mp:       make(map[int]*Node),
		capacity: capacity,
		end:      &Node{},
	}
	cache.head.nxt = cache.end
	cache.end.pre = cache.head
	return cache
}

func (c *Cache) moveToHead(node *Node) {
	if node.pre != nil {
		node.pre.nxt = node.nxt
	}
	if node.nxt != nil {
		node.nxt.pre = node.pre
	}
	node.pre = c.head
	node.nxt = c.head.nxt
	c.head.nxt.pre = node
	c.head.nxt = node
}

// Get 返回键对应的值以及该键是否存在。
func (c *Cache) Get(key int) (value int, ok bool) {
	node, ok := c.mp[key]
	if !ok {
		return 0, false
	}
	c.moveToHead(node)
	return node.val, ok
}

// Put 写入或更新一个键值对。
func (c *Cache) Put(key, value int) {
	node, ok := c.mp[key]
	if ok {
		node.val = value
		c.moveToHead(node)
		return
	} else {
		node := &Node{
			key: key,
			val: value,
		}
		c.moveToHead(node)
		c.mp[key] = node
	}
	if len(c.mp) > c.capacity {
		end := c.end.pre
		end.pre.nxt = c.end
		c.end.pre = end.pre
		delete(c.mp, end.key)
	}
}

// Len 返回当前缓存中的键数量。
func (c *Cache) Len() int {
	return len(c.mp)
}

func main() {
}
