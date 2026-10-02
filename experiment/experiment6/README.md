# Experiment 6：实现固定大小的任务池

> 类型：并发组件 · 语言：Go 1.22+ · 难度：入门进阶 · 建议时长：90～120 分钟
> 规模：核心实现约 80～150 行，连同临时验证以 500 行以内为参考，不按行数验收。

实现一个任务池：固定数量的 worker 反复接收并执行任务，关闭时等待已受理的任务完成。重点练习 goroutine 复用，以及提交和关闭之间的协作。

基础版不设置积压任务队列：所有 worker 都忙时，提交者等待。任务没有返回值；不要求任务优先级、取消、超时或 panic 恢复。

## 接口

```go
package workerpool

type Pool struct {
    // 内部结构自行设计
}

var ErrClosed error  // 固定的非 nil 错误值：任务池已关闭
var ErrNilTask error // 固定的非 nil 错误值：任务为 nil

func New(workers int) *Pool
func (p *Pool) Submit(task func()) error
func (p *Pool) Close()
```

- `workers` 合法范围为 1～1,000；大于 1,000 的输入不要求处理。
- 实例统一通过 `New` 创建；不要求零值或 nil 接收者可用，不要复制实例。公共接口保持不变。
- 最多 10,000 个并发调用，累计提交最多 100,000 个任务；调用者不修改两个公开错误变量。
- 任务最终正常返回，不会 panic、调用 `runtime.Goexit`，也不会直接或间接调用同一池的 `Submit` 或 `Close`。
- 任务自己访问共享数据时，由任务提供者负责同步。

## 要求

1. **REQ-01 创建**：`New` 创建可接收任务的池，固定使用 `workers` 个 worker；`workers <= 0` 时在调用者所在的 goroutine 中 panic，不约束文案。
2. **REQ-02 提交**：有空闲 worker 时，`Submit` 把任务交给一个 worker，并返回 nil；所有 worker 都忙时，等待空闲 worker 或关闭。以 worker 接手为“受理”；接手后，Submit 不能等待该任务结束才返回。
3. **REQ-03 执行**：每个受理任务恰好执行一次，同时执行的任务数不超过 `workers`。有足够任务时可并行执行；一个任务被阻塞不能阻止其他空闲 worker 接收任务。不要求任务执行顺序或提交者公平排队。
4. **REQ-04 关闭**：`Close` 先停止受理新任务，再等待已受理任务全部执行完、所有 worker 退出后返回。尚未受理的 Submit 结束等待并返回 ErrClosed，其任务不能执行。任务在结束前的写入在 Close 返回后可见。
5. **REQ-05 重复关闭**：`Close` 可重复、可并发调用；每次都要等到 REQ-04 的收尾完成才返回，不得 panic。关闭后提交非 nil 任务直接返回 ErrClosed。
6. **REQ-06 参数校验**：`Submit(nil)` 直接返回 ErrNilTask，不受理任务；即使已经关闭，也优先返回 ErrNilTask。两个错误值不同，允许用 `errors.Is` 判断，不约束文案。
7. **REQ-07 并发与隔离**：Submit 和 Close 可并发调用，池内共享状态没有数据竞争；不同实例互不影响。

提交与关闭竞争时，以“worker 接手任务”和“池标记关闭”的先后为准：先接手则提交成功，Close 必须等待该任务；先关闭则提交失败，任务不执行。不能只按 Submit 的调用或返回时刻判断。关闭不会打断正在执行的任务。

## 实现约束

- **CHECK-01**：只用标准库，可以使用内置 channel 和标准同步工具。必须复用固定的 worker，禁止为每个任务新建 goroutine；不得用额外队列或后台 goroutine 偷存待处理任务。通过源码检查验收。
- **CHECK-02**：等待必须使用同步机制，禁止忙等和 `time.Sleep` 轮询；关闭完成后不遗留池创建的 goroutine。通过 Review 和竞态检测检查。
- **CHECK-03**：单次任务交接的开销为 `O(1)`；池的额外空间为 `O(workers)`，不累计保存历史任务。提交者本身的等待状态、同步唤醒、任务执行、阻塞和调度开销另计。通过源码说明和 Review 检查。

## 示例

创建一个 `New(2)` 的池，并按以下条件协调调用：

| 操作 | 结果 |
| --- | --- |
| 提交任务 A、B，任务分别进入执行后等待外部信号 | 两次 Submit 均返回 nil，两个 worker 都忙 |
| 另一个 goroutine 提交任务 C | Submit 等待，C 尚未受理 |
| 释放 A，让它结束 | 一个 worker 可接手 C，第三次 Submit 返回 nil |
| C 已受理后调用 Close | 停止受理，等待 B、C 结束 |
| 释放 B、C，等它们结束 | Close 在 worker 全部退出后返回 |
| 再提交非 nil 任务，随后再次 Close | Submit 返回 ErrClosed，不执行任务；Close 直接返回 |

如果第三步释放 A 之前池就标记关闭，C 的 Submit 返回 ErrClosed，C 不执行；Close 仍需等待 A、B 结束。

## 验证要点

以下场景均需验证；只使用公开接口，临时测试文件不提交到仓库。

| 场景 | 对应要求 | 操作与期望 |
| --- | --- | --- |
| TC-01 创建与参数 | REQ-01、06 | workers 为 1、3、1000 时可执行任务并关闭；0、-1 必须 panic。打开和关闭状态下 Submit(nil) 均返回 ErrNilTask |
| TC-02 一次执行 | REQ-02、03 | 连续提交带唯一标识的任务，所有成功提交最终各执行一次；Close 返回后无遗漏、重复或额外执行 |
| TC-03 并行与限额 | REQ-02、03 | workers 为 k，前 k 个任务进入执行后均等待外部释放；它们可同时进入，第 k+1 次提交不能在释放一个任务前成功受理；整个过程同时执行数不超过 k |
| TC-04 关闭等待 | REQ-04 | 让已受理任务等待外部信号，再调用 Close；任务结束前不能关闭完成。释放任务后，Close 返回且能读到任务最终写入的结果 |
| TC-05 拒绝等待提交 | REQ-04、05 | 所有 worker 均在执行被外部信号阻塞的任务时，并发提交额外任务并关闭；额外提交均返回 ErrClosed，任务均不执行，再释放已受理任务让 Close 完成 |
| TC-06 重复关闭与竞争 | REQ-02～05、07 | 多个 Close 与非 nil Submit 并发；每个 Submit 返回 nil 或 ErrClosed，成功任务恰好执行一次，失败任务不执行；所有 Close 均能结束，之后非 nil 提交总是 ErrClosed |
| TC-07 隔离与规模 | REQ-03、07 | 一个池的任务阻塞时，另一个池可处理并关闭；另运行多生产者的大量任务，关闭后成功任务集合与实际执行集合一致，竞态检测无报告 |

并发验证用信号确认任务已经进入执行，再控制任务结束；等待使用宽松超时，失败时也要释放任务。不能把“提交 goroutine 已启动”当成“已在 Submit 中等待”，也不用固定 `Sleep` 猜测调度；TC-03～06 结合压力验证和 Review 检查交接、关闭和等待条件，worker 复用与退出通过源码检查。

## 文件与运行

手写实现放在 `experiment/experiment6/code/pool.go`，包名为 `workerpool`。只保留实现源码、README 和必要的 `go.mod`，不创建评测机或永久测试文件。

从仓库根目录初始化，module 位于 `experiment/experiment6/code/go.mod`：

```sh
mkdir -p experiment/experiment6/code
cd experiment/experiment6/code
go mod init traditionalcoding-gym/experiment6
go mod edit -go=1.22
```

写完后，在 `code/` 中执行 `gofmt -w pool.go`；`gofmt -l pool.go` 应无输出。

将 `code/` 复制到仓库外的临时目录，在副本中准备有效的测试文件，再以副本目录为工作目录运行：

```sh
go build ./...
go test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go test -race -count=1 -timeout=60s ./...
```

需要 Go 1.22+；竞态检测还需要支持 `-race` 的平台和 C 编译器。验证后清理临时文件。

完成标准：接口、所有 REQ 与 CHECK 均满足，格式化、编译和上述临时验证通过；在源码注释中简述开销、任务何时被受理和关闭如何保证收尾完成。未写实现和有效测试时，空包编译通过或 `[no test files]` 不代表完成。

可选加分：基础版完成后，增加有限容量的等待队列，并另外定义队列满时的提交行为；不影响基础验收。
