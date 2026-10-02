# Experiment 4：实现一个简易版 channel

> 类型：并发数据结构 · 语言：Go 1.22+ · 难度：入门进阶 · 建议时长：90～120 分钟
> 规模：核心实现约 80～150 行，连同临时验证以 500 行以内为参考，不按行数验收。

实现一个只能传递 `int` 的有缓冲 channel，支持发送、接收和关闭。重点练习：多个 goroutine 如何安全地共享数据，以及条件不满足时如何等待。

基础版只支持容量大于 0；不要求无缓冲 channel、`select`、超时、取消和泛型。

## 接口

```go
package minichan

type Channel struct {
    // 内部结构自行设计
}

func New(capacity int) *Channel
func (c *Channel) Send(value int)
func (c *Channel) Recv() (value int, ok bool)
func (c *Channel) Close()
```

- `capacity`：最多能暂存多少个尚未接收的值，合法范围为 1～10,000；大于 10,000 的输入不要求处理。
- `value`：任意 `int`，包括 0、负数、重复值和平台边界值。
- 实例统一通过 `New` 创建；不要求零值或 nil 接收者可用，不要复制 `Channel` 本身。
- 验证规模：最多 10,000 个并发调用，累计收发最多 100,000 个值。公共接口保持不变。

## 要求

1. **REQ-01 创建**：`New` 返回一个空的、尚未关闭的 channel；`capacity <= 0` 时 panic。
2. **REQ-02 发送**：未关闭且有空位时，`Send` 放入一个值并返回；满了就等待空位或关闭，不能丢弃或覆盖已有值。
3. **REQ-03 接收**：有值时，`Recv` 取出最早放入的值并返回 `(value, true)`；空且未关闭时等待。每个成功发送的值只能被取出一次。
4. **REQ-04 关闭与排空**：`Close` 标记关闭，不等待接收者，也不清空已有值。已有值仍按顺序接收；取完后，每次 `Recv` 都直接返回 `(0, false)`。
5. **REQ-05 关闭后的错误与唤醒**：关闭必须让等待中的调用有机会结束：接收者按 REQ-04 返回；尚未放入值的发送者 panic。关闭后调用 `Send`、重复调用 `Close` 也要 panic，且不能破坏已有数据或使后续 `Recv` 卡住。
6. **REQ-06 并发安全**：允许多个 goroutine 同时发送、接收或关闭，没有数据竞争、丢值或重复接收；不同实例互不影响。

发送与关闭同时发生时，以值实际放入 channel 和标记关闭的先后为准：先放入则发送成功，先关闭则发送 panic。并发 `Close` 只有一个成功，其余 panic。所有 panic 都发生在调用者所在的 goroutine，不约束文案。

先进先出指值放入和取出的顺序，不要求并发调用按 goroutine 启动顺序或方法返回顺序排列，也不要求等待者公平排队。

## 实现约束

- **CHECK-01**：只用标准库，可以使用 `sync` 中的同步工具。实现代码禁止使用内置 `chan`，包括用它发送通知；禁止使用 `reflect`、`unsafe` 绕过限制。通过源码检查验收。
- **CHECK-02**：等待必须使用同步机制，不能忙等或用 `time.Sleep` 轮询。提供匹配的收发操作或关闭后，不得因实现缺陷留下永久等待的 goroutine。通过代码 Review 和并发验证检查。
- **CHECK-03**：缓冲区单次读写的摊还开销为 `O(1)`，存储空间为 `O(capacity)`，不能累计保存已接收的历史数据；同步等待、唤醒及其等待者存储、调度开销另计。通过设计说明和代码 Review 检查。

## 示例

以下操作按顺序执行：

```go
c := New(2)
c.Send(10)
c.Send(0)
c.Close()

v1, ok1 := c.Recv() // 10, true
v2, ok2 := c.Recv() // 0, true：确实收到一个值 0
v3, ok3 := c.Recv() // 0, false：已经关闭且没有剩余值
```

阻塞示例：容量为 1 时，先执行 `Send(10)`。另一个 goroutine 调用 `Send(20)` 会等待；接收者取出 `10` 后，它才能放入 `20`，随后接收者可以取出 `20`。

关闭后仍能接收缓冲数据的行为与 [Go channel 的关闭规则](https://go.dev/ref/spec#Close) 一致。

## 验证要点

以下场景均需验证；只使用公开接口，临时测试文件不提交到仓库。

| 场景 | 对应要求 | 操作与期望 |
| --- | --- | --- |
| TC-01 创建与边界 | REQ-01～03 | 容量 1、3、10,000 均可填满后按顺序取出；0、负数、重复值和平台 int 边界值原样返回。容量 0、-1 必须 panic |
| TC-02 满时发送 | REQ-02、03 | 容量 1，先发送 10，再并发发送 20；取出 10 才能让第二次发送完成，下一次接收得到 `(20, true)` |
| TC-03 空时接收 | REQ-02、03 | 对空 channel 发起接收，它不能凭空返回值；发送 7 后，该接收返回 `(7, true)` |
| TC-04 关闭与排空 | REQ-04 | 空 channel 关闭后反复接收均为 `(0, false)`；另一个存有 10、0 的 channel 关闭后依次返回 `(10, true)`、`(0, true)`、`(0, false)` |
| TC-05 非法操作 | REQ-05 | 存有 8 的 channel 关闭后，发送和再次关闭均 panic；recover 后仍可收到 `(8, true)`，之后为 `(0, false)` |
| TC-06 关闭唤醒 | REQ-04、05 | 空 channel 上的多个接收者在关闭后均返回 `(0, false)`；满 channel 上的多个发送者在关闭后均 panic，原有缓冲值保留 |
| TC-07 并发收发 | REQ-02、03、06 | 多发送者、多接收者传递互不重复的整数，发送全部完成后关闭；最终收到的值与发送值完全一致，每个恰好一次，所有接收者能退出 |
| TC-08 关闭竞争与隔离 | REQ-04～06 | 多个 Close 并发时恰好一个成功；Send 与 Close 竞争时，发送成功的值可收到，panic 的值不可收到。一个实例阻塞或关闭不影响另一个实例收发 |

并发验证用同步信号协调，允许临时测试使用内置 channel。等待加宽松超时，并在失败时尽力释放等待者；预期的 panic 要在对应 goroutine 中 recover。不要用固定 `Sleep` 猜测调度，也不要把“goroutine 已启动”当成“已进入等待”；TC-02、03、06 结合并发压力验证和代码 Review 检查阻塞、唤醒逻辑。

## 文件与运行

手写实现放在 `experiment/experiment4/code/channel.go`，包名为 `minichan`。只保留实现源码、README 和必要的 `go.mod`，不创建评测机或永久测试文件。

从仓库根目录初始化，module 位于 `experiment/experiment4/code/go.mod`：

```sh
mkdir -p experiment/experiment4/code
cd experiment/experiment4/code
go mod init traditionalcoding-gym/experiment4
go mod edit -go=1.22
```

写完后，在 `code/` 中执行 `gofmt -w channel.go`；`gofmt -l channel.go` 应无输出。

编译和测试使用仓库外的临时副本。在副本中准备验证用的测试文件，再运行：

```sh
go build ./...
go test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go test -race -count=1 -timeout=60s ./...
```

需要 Go 1.22+；竞态检测还需要支持 `-race` 的平台和 C 编译器。验证后清理临时文件。

完成标准：接口、所有 REQ 与 CHECK 均满足，格式化、编译和上述临时验证通过；在源码注释中简述开销、等待条件和关闭时如何结束等待。未写实现和有效测试时，空包编译通过或 `[no test files]` 不代表完成。

可选加分：基础版完成后，扩展支持容量为 0 的无缓冲 channel；不影响基础验收。
