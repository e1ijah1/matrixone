// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memtable

import (
	"database/sql"
	"errors"

	"github.com/tidwall/btree"
)

type TableIter[
	K Ordered[K],
	V any,
] struct {
	tx       *Transaction
	iter     btree.GenericIter[*TreeItem[K, V]]
	readTime Time
}

func (t *Table[K, V, R]) NewIter(tx *Transaction) *TableIter[K, V] {
	iter := t.tree.Load().Iter()
	ret := &TableIter[K, V]{
		tx:       tx,
		iter:     iter,
		readTime: tx.Time,
	}
	ret.seekFirst()
	return ret
}

func (t *TableIter[K, V]) seekFirst() {
	var zero K
	t.iter.Seek(&TreeItem[K, V]{
		Row: &PhysicalRow[K, V]{
			Key: zero,
		},
	})
}

func (t *TableIter[K, V]) Read() (key K, value V, err error) {
	physicalRow := t.iter.Item().Row
	if physicalRow == nil {
		panic("impossible")
	}
	key = physicalRow.Key
	value, err = physicalRow.Read(t.readTime, t.tx)
	if err != nil {
		return
	}
	return
}

func (t *TableIter[K, V]) Item() (row *PhysicalRow[K, V]) {
	return t.iter.Item().Row
}

func (t *TableIter[K, V]) Next() bool {
	for {
		if ok := t.iter.Next(); !ok {
			return false
		}
		// skip unreadable values
		item := t.iter.Item()
		if item.Row == nil {
			return false
		}
		_, err := item.Row.Read(t.readTime, t.tx)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		return true
	}
}

func (t *TableIter[K, V]) First() bool {
	if ok := t.iter.First(); !ok {
		return false
	}
	t.seekFirst()
	for {
		// skip unreadable values
		item := t.iter.Item()
		if item.Row == nil {
			return false
		}
		_, err := item.Row.Read(t.readTime, t.tx)
		if errors.Is(err, sql.ErrNoRows) {
			if ok := t.iter.Next(); !ok {
				return false
			}
			continue
		}
		return true
	}
}

func (t *TableIter[K, V]) Seek(key K) bool {
	pivot := &TreeItem[K, V]{
		Row: &PhysicalRow[K, V]{
			Key: key,
		},
	}
	if ok := t.iter.Seek(pivot); !ok {
		return false
	}
	for {
		// skip unreadable values
		item := t.iter.Item()
		if item.Row == nil {
			return false
		}
		_, err := item.Row.Read(t.readTime, t.tx)
		if errors.Is(err, sql.ErrNoRows) {
			if ok := t.iter.Next(); !ok {
				return false
			}
			continue
		}
		return true
	}
}

func (t *TableIter[K, V]) Close() error {
	t.iter.Release()
	return nil
}
