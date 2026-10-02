# Experiment 10：实现一个重试工具

> 类型：函数工具 · 语言：Go 1.22+ · 难度：入门进阶 · 建议时长：60～90 分钟
> 规模：核心实现约 50～100 行，连同临时验证以 500 行以内为参考，不按行数验收。

一个操作失败后，等待一段时间再试；成功、次数耗尽或收到取消信号时结束。重点练习错误处理和 `context` 取消。

基础版使用固定重试间隔，不要求指数退避、随机抖动、错误分类或并行尝试。

## 接口

```go
package retry

import (
    "context"
    "time"
)

type WaitFunc func(context.Context, time.Duration) error

func Do(ctx context.Context, attempts int, interval time.Duration,
    fn func(context.Context) error, wait WaitFunc) error
```

- `attempts` 是最多尝试次数，包含第一次，合法范围为 1～1,000。
- `interval` 是两次尝试之间的等待时间，合法范围为 0～24 小时。更大的次数或间隔不要求处理。
- `fn` 是要执行的操作。假设它最终返回，不会 panic 或调用 `runtime.Goexit`。
- `wait == nil` 时使用默认的可取消等待；非 nil 时使用调用者的等待函数，便于自定义策略和在测试中模拟时间。
- 自定义 `wait` 最终返回 nil 表示等待结束，返回错误表示停止重试。取消无法强制打断 `fn` 或自定义 `wait`；需要它们自行配合。
- 最多并发进行 1,000 次独立的 `Do`。不同调用共用回调时，回调自身的并发安全由调用者负责。公共接口保持不变。

## 要求

1. **REQ-01 参数校验**：`ctx == nil`、`fn == nil`、`attempts <= 0` 或 `interval < 0` 时 panic，不约束文案；先校验参数，再处理取消，非法调用不能执行任何回调。
2. **REQ-02 尝试与成功**：第一次不等待；同一次 `Do` 内串行调用 `fn(ctx)`，最多调用 `attempts` 次。`fn` 返回 nil 就立即返回 nil，不再等待或调用它。
3. **REQ-03 等待与次数耗尽**：`fn` 失败且还有次数时，等待 `interval` 再试。最后一次仍失败时原样返回该次错误，不再等待；不包装错误。
4. **REQ-04 取消**：每次调用 `fn` 前检查 `ctx.Err()`，非 nil 则返回它。`fn` 返回错误后也先检查取消，再决定继续或返回最后错误。执行中的 `fn` 必须返回后才能结束 `Do`；若它返回 nil，仍按 REQ-02 返回 nil。
5. **REQ-05 等待策略**：`interval == 0` 时跳过等待函数。正间隔且 `wait != nil` 时，调用 `wait(ctx, interval)`，不再额外计时；其返回后先检查取消，再处理其错误，非 nil 错误原样返回。默认等待必须等间隔结束或被取消；取消后返回 `ctx.Err()`，不能等完整个间隔才结束。
6. **REQ-06 调用隔离**：多次 `Do` 可以并发执行，次数、结果和取消互不干扰。一次调用已结束后，不能继续执行它的回调。

取消与操作同时结束时：`fn` 成功优先；`fn` 失败后若检查到取消，则取消优先于最后一次错误。取消检查与下一次回调开始恰好重叠时，允许该次回调已开始；不要求强制中断它。

## 实现约束

- **CHECK-01**：只用标准库；不忙等，不启动脱离 `Do` 生命周期的后台任务。默认等待不能使用无法响应取消的整段 `time.Sleep`。通过源码 Review 检查。
- **CHECK-02**：实际尝试 `k` 次时，自身总开销为 `O(k)`、额外空间为 `O(1)`，不计回调、等待和运行时调度。计时资源使用结束后及时释放，不累计保留失败历史。通过设计说明和 Review 检查。

## 示例

假设 `attempts = 3`、`interval = 100 * time.Millisecond`，使用默认等待，且 `ctx` 始终未取消：

| 操作 | 结果 |
| --- | --- |
| 第一次执行 `fn` | 返回 `errTemporary` |
| 等待 100 毫秒后第二次执行 | 返回 `errTemporary` |
| 再等待 100 毫秒后第三次执行 | 返回 nil，`Do` 返回 nil |

如果三次分别返回 `errA`、`errB`、`errC`，最终返回同一个 `errC`。如果第一次就成功，只执行一次，也不等待。

## 验证要点

以下场景均为必测；仅使用公开接口，测试文件放在仓库外的临时副本中。

| 场景 | 对应要求 | 操作与期望 |
| --- | --- | --- |
| TC-01 非法参数 | REQ-01 | 分别传 nil ctx、nil fn、0 或负尝试次数、负间隔，均 panic；已取消 ctx 配非法参数仍 panic，回调计数保持 0 |
| TC-02 成功与次数 | REQ-02、03 | 首次成功只调用一次；连续失败后第三次成功共调用三次；attempts 为 1 或 1,000 且始终失败时，调用次数准确，原样返回最后错误 |
| TC-03 自定义等待 | REQ-03、05 | 正间隔、三次失败：注入记录事件的 wait，事件依次为 fn、wait、fn、wait、fn；每次 wait 收到原 ctx 和完整 interval。零间隔时 wait 从不调用 |
| TC-04 等待失败 | REQ-05 | 第一次 fn 失败后，wait 返回预先创建的 errWait：Do 原样返回该错误，不再尝试；wait 内取消 ctx 后返回 errWait，则返回 ctx.Err() |
| TC-05 取消边界 | REQ-04、05 | 进入前已取消或截止时间已过，fn 不执行，返回对应 ctx.Err()；fn 内取消并返回错误，即使次数耗尽也返回取消错误；fn 内取消但返回 nil，结果仍为 nil |
| TC-06 取消等待 | REQ-04、05 | 注入可控 wait，等它通知已进入后取消，Do 返回取消错误且无新尝试；默认等待使用 24 小时间隔，首个 fn 失败后取消，Do 能在宽松超时内结束 |
| TC-07 执行中取消 | REQ-02、04 | fn 用信号通知已进入并等待释放；此时取消 ctx，Do 仍须等 fn 返回。释放后分别验证 fn 返回 nil、普通错误时的成功/取消优先级 |
| TC-08 并发隔离 | REQ-06 | 并发执行成功、失败和取消的多次调用，各自次数与结果准确；部分调用暂停在回调中不影响其他调用完成，race 无报告 |

自定义等待测试不需要真的等待时间：用记录事件或同步信号控制即可。默认等待的完整间隔和计时资源通过 Review 检查，取消通过 TC-06 压力验证；“首个 fn 已失败”不代表已进入默认等待，不能据此断言某次运行走过计时分支。不用固定 Sleep 猜调度，各次等待加宽松超时，测试结束时释放回调并取消创建的 context。

## 文件与运行

手写实现放在 `experiment/experiment10/code/retry.go`，包名为 `retry`。仅保留实现源码、README 和必要的 `go.mod`；不创建评测机或永久测试文件。

从仓库根目录初始化，module 位于 `experiment/experiment10/code/go.mod`：

```sh
mkdir -p experiment/experiment10/code
cd experiment/experiment10/code
go mod init traditionalcoding-gym/experiment10
go mod edit -go=1.22
```

写完后，在 `code/` 中执行 `gofmt -w retry.go`；`gofmt -l retry.go` 应无输出。

将 `code/` 复制到仓库外的临时目录，加入有效断言的测试文件，在该副本的 module 根目录执行：

```sh
go build ./...
go test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go test -race -count=1 -timeout=60s ./...
```

需要 Go 1.22+；race 检测还需要支持的平台和 C 编译器。验证后清理临时文件。

完成标准：公共接口、REQ 与 CHECK 均满足，格式化、编译和所有必测场景通过；源码注释说明取消与错误的优先级、等待资源的释放和复杂度。没有实现和有效测试时，空包编译通过或 `[no test files]` 不代表完成。

可选加分：支持指数退避，为等待间隔设置增长倍数和上限；不影响基础验收。
