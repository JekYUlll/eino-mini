package session

import (
	"os"
	"strconv"
)

func getIntEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// Prune 会：
// 1) 永远保留最早的 system（如果存在）
// 2) 只保留最后 N 轮（1轮 = user+assistant；不完整的一轮也算）
// 3) 再按字符数上限裁剪最早的对话内容
func Prune(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}

	maxTurns := getIntEnv("CHAT_MAX_TURNS", 10)
	maxChars := getIntEnv("CHAT_MAX_CHARS", 24000)

	// 1) 找 system（我们约定 system 只放第一条）
	var system *Message
	start := 0
	if msgs[0].Role == "system" {
		system = &msgs[0]
		start = 1
	}

	rest := msgs[start:]

	// 2) 轮次裁剪：保留最后 maxTurns 轮
	// 将 rest 从尾部往前数，遇到 user 认为是新一轮开始
	turnStarts := make([]int, 0, maxTurns+1)
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i].Role == "user" {
			turnStarts = append(turnStarts, i)
			if len(turnStarts) >= maxTurns {
				break
			}
		}
	}
	// 如果 user 条数不足，说明总轮数 <= maxTurns，不裁剪
	cut := 0
	if len(turnStarts) >= maxTurns {
		// turnStarts[0] 是最后一轮的 user，下标越大越早
		// 我们要保留“最早的那轮”的起点
		cut = turnStarts[len(turnStarts)-1]
	}

	kept := rest
	if cut > 0 && cut < len(rest) {
		kept = rest[cut:]
	}

	// 3) 字符数裁剪（第二道保险）：从后往前累加，超过就截掉更早的
	total := 0
	idx := len(kept)
	for i := len(kept) - 1; i >= 0; i-- {
		total += len(kept[i].Content)
		if total > maxChars {
			idx = i + 1
			break
		}
	}
	if idx < len(kept) {
		kept = kept[idx:]
	}

	// 4) 拼回 system
	if system != nil {
		out := make([]Message, 0, 1+len(kept))
		out = append(out, *system)
		out = append(out, kept...)
		return out
	}
	return kept
}

// prunePlan returns whether to keep the leading system message and the tail start index
// (absolute index in msgs) to keep after trimming.
func prunePlan(msgs []Message) (bool, int) {
	if len(msgs) == 0 {
		return false, 0
	}

	maxTurns := getIntEnv("CHAT_MAX_TURNS", 10)
	maxChars := getIntEnv("CHAT_MAX_CHARS", 24000)

	keepSystem := false
	start := 0
	if msgs[0].Role == "system" {
		keepSystem = true
		start = 1
	}

	rest := msgs[start:]

	// 2) 轮次裁剪：保留最后 maxTurns 轮
	turnStarts := make([]int, 0, maxTurns+1)
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i].Role == "user" {
			turnStarts = append(turnStarts, i)
			if len(turnStarts) >= maxTurns {
				break
			}
		}
	}
	cut := 0
	if len(turnStarts) >= maxTurns {
		cut = turnStarts[len(turnStarts)-1]
	}

	absStart := start + cut
	kept := rest
	if cut > 0 && cut < len(rest) {
		kept = rest[cut:]
	}

	// 3) 字符数裁剪：从后往前累加，超过就截掉更早的
	total := 0
	idx := len(kept)
	for i := len(kept) - 1; i >= 0; i-- {
		total += len(kept[i].Content)
		if total > maxChars {
			idx = i + 1
			break
		}
	}

	tailStart := absStart
	if idx < len(kept) {
		tailStart = absStart + idx
	}
	return keepSystem, tailStart
}
