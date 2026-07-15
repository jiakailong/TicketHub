package cache

import "sync"

const defaultLockStripes = 256

type StripedRWMutex struct {
	stripes []sync.RWMutex
}

func NewStripedRWMutex(size int) *StripedRWMutex {
	if size <= 0 {
		size = defaultLockStripes
	}
	return &StripedRWMutex{stripes: make([]sync.RWMutex, size)}
}

func (s *StripedRWMutex) For(key string) *sync.RWMutex {
	var hash uint64 = 14695981039346656037
	for index := 0; index < len(key); index++ {
		hash ^= uint64(key[index])
		hash *= 1099511628211
	}
	return &s.stripes[hash%uint64(len(s.stripes))]
}
