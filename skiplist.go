package main

import (
	"math/rand"
	"time"
)

const (
	maxLevel    = 16
	probability = 0.5
)

type Skiplist struct {
	Head  *Node
	Level int
	rng   *rand.Rand
	Size  int
}

type Node struct {
	Key   string
	Value string
	Next  []*Node
}

func NewSkiplist() *Skiplist {
	src := rand.NewSource(time.Now().UnixNano())
	return &Skiplist{
		Head:  &Node{Next: make([]*Node, 0)},
		Level: 0,
		rng:   rand.New(src),
	}
}

func (sl *Skiplist) RandomLevel() int {
	lvl := 0
	for sl.rng.Float64() < 0.5 && lvl < maxLevel-1 {
		lvl++
	}
	return lvl
}

func (sl *Skiplist) Insert(key, value string) {
	update := make([]*Node, maxLevel)
	current := sl.Head

	for i := sl.Level; i >= 0; i-- {
		for current.Next[i] != nil && current.Next[i].Key < key {
			current = current.Next[i]
		}
		update[i] = current
	}
	current = current.Next[0]
	if current != nil && current.Key == key {
		current.Value = value
		return
	}

	lvl := sl.RandomLevel()
	if lvl > sl.Level {

		for i := sl.Level + 1; i <= lvl; i++ {
			update[i] = sl.Head
		}
		sl.Level = lvl
	}
	newNode := &Node{
		Key:   key,
		Value: value,
		Next:  make([]*Node, lvl+1),
	}

	for i := 0; i <= lvl; i++ {
		newNode.Next[i] = update[i].Next[i]
		update[i].Next[i] = newNode
	}
	sl.Size++
}

func (sl *Skiplist) Search(key string) (string, bool) {
	current := sl.Head

	for i := sl.Level; i >= 0; i-- {
		for current.Next[i] != nil && current.Next[i].Key < key {
			current = current.Next[i]
		}
	}
	current = current.Next[0]
	if current != nil && current.Key == key {
		return current.Value, true
	}
	return "", false
}

type Iterator struct {
	current *Node
}

func (sl *Skiplist) NewIterator() *Iterator {
	return &Iterator{current: sl.Head}
}

func (it *Iterator) Next() bool {
	if it.current == nil {
		return false
	}
	it.current = it.current.Next[0]
	return it.current != nil
}

func (it *Iterator) Key() string {
	return it.Key()
}

func (it *Iterator) Value() string {
	return it.Value()
}
