package gui

// memo 缓存一个值：键和上次一样就直接用，键变了才重新算。逐帧要问、算起来不便宜的值，
// 如缓冲区有没有改过、按查询排好的路径，用它避免每帧重算。
type memo[K comparable, V any] struct {
	key K
	val V
	ok  bool
}

// get 返回键为 key 时的值。
func (m *memo[K, V]) get(key K, compute func() V) V {
	if !m.ok || m.key != key {
		m.key, m.val, m.ok = key, compute(), true
	}
	return m.val
}

// forget 让缓存的值作废，下一次 get 重新算。
func (m *memo[K, V]) forget() { m.ok = false }
