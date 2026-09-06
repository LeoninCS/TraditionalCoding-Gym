package main

import (
	"fmt"
	"os"
)

type judgeStep struct {
	kind              string
	cache, key, value int
	ok                bool
}

type judgeCase struct {
	name  string
	steps []judgeStep
}

// judge 通过公开 API 执行基础测试；失败只显示原因，不显示操作位置或调用栈。
// 单组断言失败或 panic 不影响其余用例，任一组失败最终退出码为 1。
// 复杂度、空间上界及禁用组件仍需代码 Review；这里不测试并发或其他加分项。
func judge() {
	cases := judgeCases()
	failed := 0
	for i, tc := range cases {
		func() {
			passed := false
			reason := ""
			defer func() {
				problem := recover()
				if !passed {
					if reason == "" {
						reason = fmt.Sprintf("运行异常：%v", problem)
					}
					failed++
					fmt.Printf("[失败] %d. %s\n  原因：%s\n", i+1, tc.name, reason)
				} else {
					fmt.Printf("[通过] %d. %s\n", i+1, tc.name)
				}
			}()
			caches := make(map[int]*Cache)
			for _, s := range tc.steps {
				switch s.kind {
				case "New":
					caches[s.cache] = New(s.value)
				case "Put":
					caches[s.cache].Put(s.key, s.value)
				case "Get":
					value, ok := caches[s.cache].Get(s.key)
					if value != s.value || ok != s.ok {
						reason = fmt.Sprintf("返回结果不符：期望 (%d, %t)，实际 (%d, %t)", s.value, s.ok, value, ok)
						return
					}
				case "Len":
					if got := caches[s.cache].Len(); got != s.value {
						reason = fmt.Sprintf("长度不符：期望 %d，实际 %d", s.value, got)
						return
					}
				default:
					reason = "测试配置错误：未知操作类型"
					return
				}
			}
			passed = true
		}()
	}
	fmt.Printf("\n共 %d 组：通过 %d 组，失败 %d 组\n", len(cases), len(cases)-failed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// judgeCases 的期望只来自题面和固定序列，不读取私有字段、不包含缓存参考实现。
// get 是一次真实读取，会改变使用顺序；不会在检查之间隐式插入额外读取。
func judgeCases() []judgeCase {
	newCache := func(capacity int) judgeStep { return judgeStep{kind: "New", value: capacity} }
	put := func(key, value int) judgeStep { return judgeStep{kind: "Put", key: key, value: value} }
	get := func(key, value int, ok bool) judgeStep {
		return judgeStep{kind: "Get", key: key, value: value, ok: ok}
	}
	length := func(want int) judgeStep { return judgeStep{kind: "Len", value: want} }
	on := func(cache int, s judgeStep) judgeStep { s.cache = cache; return s }
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	cases := []judgeCase{
		{"单次写入和读取", []judgeStep{
			newCache(1), put(1, 10), get(1, 10, true), length(1),
		}},
		{"基本读写、未满容量更新", []judgeStep{
			newCache(3), length(0), get(1, 0, false), put(1, 10), length(1),
			put(1, 11), length(1), put(2, 20), length(2), put(1, 12), length(2),
			put(3, 30), length(3), get(1, 12, true), get(2, 20, true), get(3, 30, true),
		}},
		{"容量 1：更新、淘汰和重插", []judgeStep{
			newCache(1), put(1, 10), put(1, 11), length(1), get(1, 11, true),
			put(2, 20), length(1), get(1, 0, false), get(2, 20, true),
			put(1, 100), length(1), get(2, 0, false), get(1, 100, true),
		}},
		{"容量 1：读取后继续写入", []judgeStep{
			newCache(1), put(1, 10), get(1, 10, true), put(2, 20),
			get(1, 0, false), get(2, 20, true), length(1),
		}},
		{"容量 1：更新后继续写入", []judgeStep{
			newCache(1), put(1, 10), put(1, 11), put(2, 20),
			get(1, 0, false), get(2, 20, true), length(1),
		}},
		{"题面完整示例", []judgeStep{
			newCache(2), put(1, 10), put(2, 20), get(1, 10, true), put(3, 30),
			get(2, 0, false), put(1, 11), put(4, 40), length(2),
			get(2, 0, false), get(3, 0, false), get(1, 11, true), get(4, 40, true),
		}},
		{"未命中不刷新顺序，包括查询已淘汰的键", []judgeStep{
			newCache(3), put(1, 10), put(2, 20), put(3, 30), get(1, 10, true),
			get(99, 0, false), get(-1, 0, false), get(0, 0, false), get(99, 0, false),
			put(4, 40), get(2, 0, false), get(2, 0, false), put(5, 50),
			get(3, 0, false), get(1, 10, true), get(4, 40, true), get(5, 50, true), length(3),
		}},
		{"Len 不刷新顺序", []judgeStep{
			newCache(4), put(1, 10), put(2, 20), put(3, 30), put(4, 40), get(2, 20, true),
			length(4), length(4), length(4), put(5, 50), get(1, 0, false),
			length(4), length(4), put(6, 60), get(3, 0, false),
			get(2, 20, true), get(4, 40, true), get(5, 50, true), get(6, 60, true), length(4),
		}},
		{"零和负数键值：存在的零值必须返回 true", []judgeStep{
			newCache(2), put(0, 0), put(-1, -10), get(-1, -10, true), get(0, 0, true),
			get(-2, 0, false), put(2, 0), get(-1, 0, false),
			get(0, 0, true), get(2, 0, true), length(2),
		}},
		{"不同键可以具有相同值，淘汰按键区分", []judgeStep{
			newCache(2), put(1, 99), put(2, 99), put(3, 99),
			get(1, 0, false), get(2, 99, true), get(3, 99, true), length(2),
			put(4, 99), get(2, 0, false), get(3, 99, true), get(4, 99, true), length(2),
		}},
		{"int 极值：不同键和值不能混淆或截断", []judgeStep{
			newCache(3), put(minInt, maxInt), put(maxInt, minInt), put(0, 0),
			get(minInt, maxInt, true), get(maxInt, minInt, true), get(0, 0, true), length(3),
		}},
		{"int 极值键更新为零后仍参与淘汰", []judgeStep{
			newCache(2), put(minInt, maxInt), put(maxInt, minInt), put(minInt, 0),
			put(0, -1), get(maxInt, 0, false), get(minInt, 0, true), get(0, -1, true), length(2),
		}},
		{"连续淘汰、重插已淘汰键、再更新已有键", []judgeStep{
			newCache(3), put(1, 10), length(1), put(2, 20), length(2), put(3, 30), length(3),
			put(4, 40), length(3), put(5, 50), length(3), put(1, 100), length(3),
			put(4, 400), length(3), put(6, 60), length(3), get(2, 0, false),
			get(3, 0, false), get(5, 0, false), get(1, 100, true), get(4, 400, true), get(6, 60, true),
		}},
		{"三个实例的值、容量和顺序互不影响", []judgeStep{
			newCache(2), put(1, 10), put(2, 20), on(1, newCache(1)), on(1, length(0)), length(2),
			get(1, 10, true), on(1, put(1, 100)), put(3, 30), get(2, 0, false),
			get(1, 10, true), on(1, get(1, 100, true)), on(1, put(1, 101)), get(1, 10, true),
			on(1, put(2, 200)), on(1, get(1, 0, false)), on(1, get(2, 200, true)),
			on(2, newCache(0)), on(2, put(1, 999)), on(2, get(1, 0, false)), on(2, length(0)),
			get(1, 10, true), get(3, 30, true), length(2), on(1, get(2, 200, true)), on(1, length(1)),
		}},
	}

	// 未满时更新也必须刷新顺序；在触发淘汰前不能用 Get 提前刷新它。
	for _, value := range []int{10, 11} {
		cases = append(cases, judgeCase{fmt.Sprintf("未满容量更新刷新顺序 value=%d", value), []judgeStep{
			newCache(3), put(1, 10), put(2, 20), put(1, value), length(2),
			put(3, 30), put(4, 40), get(2, 0, false),
			get(1, value, true), get(3, 30, true), get(4, 40, true), length(3),
		}})
	}

	// 每个容量独立运行，某组异常不会跳过另一个容量。
	for _, capacity := range []int{0, 1, 2, 7, 64} {
		cases = append(cases, judgeCase{fmt.Sprintf("空缓存 capacity=%d", capacity), []judgeStep{
			newCache(capacity), length(0), get(0, 0, false), get(-1, 0, false),
			get(minInt, 0, false), get(maxInt, 0, false), length(0),
		}})
	}
	zero := []judgeStep{newCache(0)}
	for _, pair := range [][2]int{{0, 0}, {0, -1}, {-1, 0}, {minInt, maxInt}, {maxInt, minInt}, {1, 1}} {
		zero = append(zero, put(pair[0], pair[1]), length(0), get(pair[0], 0, false))
	}
	cases = append(cases, judgeCase{"容量 0：重复写入和特殊整数始终无效", zero})

	// cap=4，原键按 1、2、3、4 写入；触达 target 后，原键从最旧到最新的顺序。
	// 48 个组合覆盖最旧/中间/最新键，以及同值/改值更新。
	// 更新后先完成所有插入，再读取验值，避免 Get 掩盖 Put 未刷新顺序的问题。
	evictions := [4][4]int{{2, 3, 4, 1}, {1, 3, 4, 2}, {1, 2, 4, 3}, {1, 2, 3, 4}}
	for target := 1; target <= 4; target++ {
		for _, mode := range []string{"读取", "同值更新", "改值更新"} {
			for added := 1; added <= 4; added++ {
				steps := []judgeStep{newCache(4), put(1, 10), put(2, 20), put(3, 30), put(4, 40)}
				value := target * 10
				if mode == "读取" {
					steps = append(steps, get(target, value, true))
				} else {
					if mode == "改值更新" {
						value++
					}
					steps = append(steps, put(target, value))
				}
				for key := 5; key < 5+added; key++ {
					steps = append(steps, put(key, key*10))
				}
				steps = append(steps, length(4))
				for key := 1; key <= 4; key++ {
					present := true
					for _, evicted := range evictions[target-1][:added] {
						if key == evicted {
							present = false
						}
					}
					want := key * 10
					if key == target {
						want = value
					}
					if !present {
						want = 0
					}
					steps = append(steps, get(key, want, present))
				}
				for key := 5; key < 5+added; key++ {
					steps = append(steps, get(key, key*10, true))
				}
				steps = append(steps, length(4))
				cases = append(cases, judgeCase{fmt.Sprintf("%s target=%d，随后插入 %d 个新键", mode, target, added), steps})
			}
		}
	}

	for _, capacity := range []int{1, 2, 4} {
		for _, mode := range []string{"读取", "同值更新"} {
			steps := []judgeStep{newCache(capacity)}
			for key := 1; key <= capacity; key++ {
				steps = append(steps, put(key, key*10))
			}
			for i := 0; i < 64; i++ {
				if mode == "读取" {
					steps = append(steps, get(1, 10, true))
				} else {
					steps = append(steps, put(1, 10))
				}
				steps = append(steps, length(capacity))
			}
			for key := 2; key <= capacity+1; key++ {
				steps = append(steps, put(key+capacity, key))
			}
			for key := 1; key <= capacity; key++ {
				steps = append(steps, get(key, 0, false))
			}
			for key := 2; key <= capacity+1; key++ {
				steps = append(steps, get(key+capacity, key, true))
			}
			steps = append(steps, length(capacity))
			cases = append(cases, judgeCase{fmt.Sprintf("重复%s 64 次后全部换新 capacity=%d", mode, capacity), steps})
		}
	}

	// 以下固定序列的期望由保留键的区间公式给出，不使用随机数或参考缓存。
	const rounds = 256
	for _, capacity := range []int{1, 2, 3, 7, 64} {
		steps := []judgeStep{newCache(capacity)}
		for key := 0; key < rounds; key++ {
			wantLen := key + 1
			if wantLen > capacity {
				wantLen = capacity
			}
			steps = append(steps, put(key, key*7-3), length(wantLen), put(key, key*7+1),
				length(wantLen), get(key, key*7+1, true), get(-1, 0, false))
			if key >= capacity {
				steps = append(steps, get(key-capacity, 0, false))
			}
		}
		for key := 0; key < rounds; key++ {
			if key < rounds-capacity {
				steps = append(steps, get(key, 0, false))
			} else {
				steps = append(steps, get(key, key*7+1, true))
			}
		}
		steps = append(steps, length(capacity))
		cases = append(cases, judgeCase{fmt.Sprintf("256 轮递增键写入与更新 capacity=%d", capacity), steps})
	}
	for _, capacity := range []int{2, 3, 7, 64} {
		steps := []judgeStep{newCache(capacity), put(0, -1)}
		for key := 1; key <= rounds; key++ {
			wantLen := key + 1
			if wantLen > capacity {
				wantLen = capacity
			}
			steps = append(steps, put(key, key*11), length(wantLen), get(0, -1, true))
			if key >= capacity {
				steps = append(steps, get(key-capacity+1, 0, false))
			}
		}
		steps = append(steps, get(0, -1, true))
		for key := 1; key <= rounds; key++ {
			if key <= rounds-capacity+1 {
				steps = append(steps, get(key, 0, false))
			} else {
				steps = append(steps, get(key, key*11, true))
			}
		}
		steps = append(steps, length(capacity))
		cases = append(cases, judgeCase{fmt.Sprintf("256 轮持续读取热点键 capacity=%d", capacity), steps})
	}
	cycle := []judgeStep{newCache(3)}
	for i := 1; i <= 300; i++ {
		wantLen := i
		if wantLen > 3 {
			wantLen = 3
		}
		cycle = append(cycle, put(i%5, i), length(wantLen))
	}
	cycle = append(cycle, get(1, 0, false), get(2, 0, false), get(0, 300, true), get(3, 298, true), get(4, 299, true), length(3))
	cases = append(cases, judgeCase{"300 轮循环键淘汰与重插", cycle})
	return cases
}
