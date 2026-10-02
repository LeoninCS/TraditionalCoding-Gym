# Experiment 7：带过期时间的缓存

> 类型：并发数据结构 · 语言：Go 1.22+ · 难度：入门进阶 · 建议时长：60～90 分钟
> 规模：核心实现约 60～100 行，连同临时验证以 500 行以内为参考，不按行数验收。

实现一个保存 `string → int` 的缓存，每次写入可以设置存活时间（TTL）。重点练习并发读写和过期处理。

基础版支持写入、读取、删除和手动清理；不要求容量限制、LRU、持久化或后台定时清理。

## 接口

```go
package ttlcache

import "time"

type Cache struct {
    // 内部结构自行设计
}

func New(now func() time.Time) *Cache
func (c *Cache) Set(key string, value int, ttl time.Duration)
func (c *Cache) Get(key string) (value int, ok bool)
func (c *Cache) Delete(key string)
func (c *Cache) Cleanup() int
```

- `key` 区分大小写，允许空字符串，长度不超过 128 字节；`value` 可以是任意 `int`。
- `now` 用来获取当前时间；传 nil 时使用 `time.Now`。测试可以传入可控制的时钟，不必真的等待过期。
- 注入的时钟由调用者保证并发安全、时间不回退，且快速返回、不 panic、不回调同一缓存；时间加减不存在溢出。
- 实例通过 `New` 创建，不要求零值或 nil 接收者可用，不要复制实例。累计操作最多 100,000 次，同时存储不超过 10,000 个 key；公共接口保持不变。

## 要求

1. **REQ-01 创建**：`New` 返回空缓存；普通 key 和空 key 的读取均返回 `(0, false)`。nil 时钟参数可用。
2. **REQ-02 写入与更新**：`ttl > 0` 时保存值，到期时刻为本次 `Set` 读取的当前时间加上 ttl。同 key 再次写入必须同时覆盖旧值和旧到期时刻；`ttl <= 0` 等同于删除该 key，不留下新值。
3. **REQ-03 读取与过期**：读取时，当前时间严格早于到期时刻才返回 `(value, true)`。不存在或已经到期都返回 `(0, false)`；读到过期条目时将它删除。读取不会延长有效期。
4. **REQ-04 删除**：`Delete` 删除对应条目，无论它是否过期；删除不存在的 key 不做任何事，不影响其他 key。
5. **REQ-05 手动清理**：`Cleanup` 以本次读取的当前时间清理所有已到期条目，返回实际删除的条目数，保留仍有效的条目。已经被 `Get` 或 `Delete` 删除的条目不再计数。
6. **REQ-06 并发与隔离**：同一实例的所有方法可以并发调用，没有数据竞争；结果等价于某个符合调用先后关系的串行顺序。过期读取或清理不能依据旧到期时间删除已更新的新值，不同实例互不影响。

到期边界是 `当前时间 >= 到期时刻`。允许尚未读取、尚未清理的过期条目暂时占用空间；不允许把它们当作有效值返回。

## 实现约束

- **CHECK-01**：只用标准库，不启动后台 goroutine，不用忙等或 `time.Sleep` 实现过期；所有时间判断都使用传入的时钟。通过源码检查和竞态检测验收。
- **CHECK-02**：`Set`、`Get`、`Delete` 平均时间为 `O(1)`，`Cleanup` 为 `O(n)`，n 是当前存储的条目数，不计同步等待和时钟调用。条目存储为 `O(n)`，删除后不保留历史 key/value；不要求收缩容器底层容量。通过源码注释和代码 Review 检查。

## 示例

假时钟从 `t0` 开始，以下操作按顺序执行：

| 操作 | 结果 |
| --- | --- |
| `Set("a", 10, 2*time.Second)` | a 在 t0 + 2 秒到期 |
| 时间推进到 t0 + 1 秒，`Get("a")` | `(10, true)` |
| `Set("a", 20, 3*time.Second)` | 更新为 20，新的到期时刻是 t0 + 4 秒 |
| 时间推进到 t0 + 2 秒，`Cleanup()` | `0`，a 的旧到期时间已失效 |
| `Get("a")` | `(20, true)` |
| 时间推进到 t0 + 4 秒，`Get("a")` | `(0, false)`，并删除过期条目 |
| `Cleanup()` | `0`，该条目已经被读取操作删除 |

## 验证要点

以下场景均需验证；测试只使用公开接口，临时测试文件不提交到仓库。

| 场景 | 对应要求 | 操作与期望 |
| --- | --- | --- |
| TC-01 创建与基本值 | REQ-01～03 | 新缓存读取普通 key、空 key 均未命中；写入后原样返回 0、负数和平台 int 边界值，且 ok 为 true。`New(nil)` 可以正常创建和查询空缓存 |
| TC-02 到期边界 | REQ-02、03 | 写入 TTL 为 1 秒的值；到期前 1 纳秒仍命中，恰好到期和到期后均为 `(0, false)`；中途重复读取不延长 TTL |
| TC-03 更新 | REQ-02、03、05 | 按示例延长 TTL，旧到期时刻清理不删除新值；另验证缩短 TTL、覆盖已过期值，以及不同 key 分别按各自时间过期 |
| TC-04 非正 TTL 与删除 | REQ-02～04 | 对已存在和不存在的 key 分别执行 TTL 为 0、负数的 Set 和重复 Delete；目标均未命中，其他有效 key 保留 |
| TC-05 清理计数 | REQ-03～05 | 固定时间下设置 3 个短 TTL 和 1 个长 TTL；推进到短 TTL 到期，先 Get 删除一个、Delete 删除一个，Cleanup 返回 1，再清理返回 0，长 TTL 值仍命中；空缓存清理返回 0 |
| TC-06 并发与隔离 | REQ-02～06 | 固定假时钟，各 goroutine 写入并读取自己的 key，再删除后确认未命中；同 key 混合读写清理无数据竞争，读取只可未命中或返回已写入值，结束后最后一次串行 Set 的值可读。独立缓存同名 key 不互相影响；更新与清理的竞争顺序结合 Review 检查 |
| TC-07 规模与清理 | REQ-02、03、05 | 写入 10,000 个不同 key，按各自值读取；统一推进到到期时刻，Cleanup 返回 10,000，之后全部未命中 |

假时钟只在上一批操作全部结束后推进，并发操作期间固定时间；不用真实等待或 `Sleep` 判断到期。并发测试加宽松超时，不能把 goroutine 启动顺序当作实际生效顺序。

## 文件与运行

手写实现放在 `experiment/experiment7/code/cache.go`，包名为 `ttlcache`。只保留实现源码、README 和必要的 `go.mod`，不创建评测机或永久测试文件。

从仓库根目录初始化，module 位于 `experiment/experiment7/code/go.mod`：

```sh
mkdir -p experiment/experiment7/code
cd experiment/experiment7/code
go mod init traditionalcoding-gym/experiment7
go mod edit -go=1.22
```

写完后，在 `code/` 中执行 `gofmt -w cache.go`；`gofmt -l cache.go` 应无输出。

编译和测试使用仓库外的临时副本。在副本中准备有效断言的测试文件，再运行：

```sh
go build ./...
go test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go test -race -count=1 -timeout=60s ./...
```

需要 Go 1.22+；竞态检测还需要支持 `-race` 的平台和 C 编译器。验证后清理临时文件。

完成标准：接口、所有 REQ 与 CHECK 均满足，格式化、编译和临时验证通过；在源码注释中简述复杂度和更新、清理如何保持一致。未写实现和有效测试时，空包编译通过或 `[no test files]` 不代表完成。

可选加分：基础版完成后，增加可停止的后台定时清理；不影响基础验收。
