# Experiment 2：限并发、保序的批处理函数

用 Go 实现一个 `Map` 函数：并发处理一组整数，限制同时执行的任务数，并按输入顺序返回结果。练习重点是 goroutine、并发控制和结果收集。

## 接口

```go
package parallelmap

type Result struct {
    Value int
    Err   error
}

func Map(input []int, workers int, fn func(int) (int, error)) []Result
```

- `input`：待处理的整数，最多 10,000 个，允许重复值。
- `workers`：同时执行的任务数上限，可以大于输入长度。
- `fn`：处理一个整数，返回结果和错误。假设它支持并发调用、最终会返回且不会 panic。

## 要求

1. `workers <= 0` 或 `fn == nil` 时，在调用任何 `fn` 前直接 panic；空输入也要校验。
2. 参数合法时，空输入返回 `nil`，不调用 `fn`。
3. 每个输入位置恰好处理一次。某个任务失败时，其他任务继续执行。
4. 返回结果与输入一一对应、顺序相同，完整保留 `fn` 返回的值和错误；出错时也可能有非零返回值。
5. 最多同时执行 `min(workers, len(input))` 个任务。有空闲名额和待处理任务时，要能继续启动任务，不能全部串行处理。
6. 等全部任务完成后再返回，内部 goroutine 随之退出，不再修改返回结果。
7. 不修改输入；多次调用或同时调用 `Map` 时，各次调用的并发限制和结果互不影响，也不能互相阻塞。

实现只用标准库。goroutine 数量应随并发上限增长，不要为每个输入都创建一个 goroutine 再限流。用同步机制等待任务，不要轮询或用 `time.Sleep` 等待。

设 `n` 为输入数量、`k = min(workers, n)`：任务分配和结果收集的总工作量应为 `O(n)`，额外空间为 `O(n+k)`（不计 `fn` 自身开销）；空输入不创建 goroutine。

调用期间，调用者和 `fn` 都不要修改输入。无需实现取消、超时、重试或 panic 恢复。

## 示例

输入 `[]int{3, -1, 2}`，`workers = 2`。假设 `fn` 对非负数返回平方，对负数返回 `-99` 和错误 `errNegative`：

```text
输入： [3, -1, 2]
结果： [{9, nil}, {-99, errNegative}, {4, nil}]
```

任务可以先后完成，但结果始终按输入顺序排列。负数任务出错，不影响另外两个任务。

## 文件与运行

使用 Go 1.22 或更高版本，在 `experiment/experiment2/code/` 中手写实现：

```text
code/
├── go.mod        # module: traditionalcoding-gym/experiment2，go: 1.22
└── map.go        # 包名 parallelmap
```

实验目录只保留 README、实现源码和必要的构建配置，不保留测试文件、评测脚本、报告或编译产物。

从仓库根目录初始化：

```sh
mkdir -p experiment/experiment2/code
cd experiment/experiment2/code
go mod init traditionalcoding-gym/experiment2
go mod edit -go=1.22
```

写完后，在 `code/` 中格式化：

```sh
gofmt -w map.go
```

编译和测试使用临时副本，验证用的测试文件也只放在临时目录。准备好临时副本和测试后，在其中运行：

```sh
go build ./...
go test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go test -race -count=1 -timeout=60s ./...
```

竞态检测需要支持 `-race` 的平台和 C 编译器。

## 测试要点

- 非法参数会 panic，且不执行任务；合法空输入返回 `nil`。
- 普通输入、重复值、负数和 `int` 边界值都能正确处理，输入保持不变。
- 部分失败或全部失败时，每个任务仍只执行一次，结果和错误原样保留。
- 即使任务完成顺序不同，结果仍有序，且所有任务完成后才返回。
- `workers = 1`、正常并发和 `workers` 远大于输入数量时都正确；实际并发数达到但不超过上限。
- 多次调用和同时调用互不影响；某次调用被阻塞时，另一次仍能完成，旧结果也不会被改写。
- 10,000 个输入没有遗漏、重复执行或结果错位。

并发测试用 channel 控制任务的进入和释放，不用 `time.Sleep` 猜执行顺序；等待时加超时保护，失败时也要释放阻塞的任务。测试中的共享计数也要正确同步。

完成标准：在临时副本中验证上述场景，编译、测试和竞态检测通过；在 `map.go` 注释中简单说明复杂度、如何限制并发，以及如何等待任务和 goroutine 结束。不要求提交测试文件。
