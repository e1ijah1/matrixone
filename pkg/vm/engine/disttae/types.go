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

package disttae

import (
	"context"
	"sync"

	"github.com/matrixorigin/matrixone/pkg/container/batch"
	"github.com/matrixorigin/matrixone/pkg/pb/api"
	"github.com/matrixorigin/matrixone/pkg/pb/logservice"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/pb/timestamp"
	"github.com/matrixorigin/matrixone/pkg/pb/txn"
	"github.com/matrixorigin/matrixone/pkg/txn/client"
	"github.com/matrixorigin/matrixone/pkg/vm/engine"
	"github.com/matrixorigin/matrixone/pkg/vm/mheap"
)

const (
	INSERT = iota
	DELETE
)

type DNStore = logservice.DNStore

// tae's block metadata, which is currently just an empty one,
// does not serve any purpose When tae submits a concrete structure,
// it will replace this structure with tae's code
type BlockMeta struct {
}

// Cache is a multi-version cache for maintaining some table data.
// The cache is concurrently secure,  with multiple transactions accessing
// the cache at the same time.
// For different dn,  the cache is handled independently, the format
// for our example is k-v, k being the dn number and v the timestamp,
// suppose there are 2 dn, for table A exist dn0 - 100, dn1 - 200.
type Cache interface {
	// update table's cache to the specified timestamp
	Update(ctx context.Context, dnList []DNStore, databaseId uint64,
		tableId uint64, ts timestamp.Timestamp) error
	// BlockList return a list of unmodified blocks that do not require
	// a merge read and can be very simply distributed to other nodes
	// to perform queries
	BlockList(ctx context.Context, dnList []DNStore, databaseId uint64,
		tableId uint64, ts timestamp.Timestamp, entries [][]Entry) []BlockMeta
	// NewReader create some readers to read the data of the modified blocks,
	// including workspace data
	NewReader(ctx context.Context, readerNumber int, expr *plan.Expr,
		dnList []DNStore, databaseId uint64, tableId uint64,
		ts timestamp.Timestamp, entries [][]Entry) ([]engine.Reader, error)
}

// mvcc is the core data structure of cn and is used to
// maintain multiple versions of logtail data for a table's partition
type MVCC interface {
	CheckPoint(ts timestamp.Timestamp) error
	Insert(ctx context.Context, bat *api.Batch) error
	Delete(ctx context.Context, bat *api.Batch) error
	BlockList(ctx context.Context, ts timestamp.Timestamp,
		entries [][]Entry) []BlockMeta
	NewReader(ctx context.Context, readerNumber int, expr *plan.Expr,
		ts timestamp.Timestamp, entries [][]Entry) ([]engine.Reader, error)
}

type Engine struct {
	sync.RWMutex
	db                *DB
	m                 *mheap.Mheap
	cli               client.TxnClient
	getClusterDetails GetClusterDetailsFunc
	txns              map[string]*Transaction
}

// DB is implementataion of cache
type DB struct {
	sync.RWMutex
	dnMap  map[string]int
	cli    client.TxnClient
	tables map[[2]uint64]Partitions
}

type Partitions []*Partition

// a partition corresponds to a dn
type Partition struct {
	sync.RWMutex
	// multi-version data of logtail, implemented with reusee's memengine
	data MVCC
	// last updated timestamp
	ts timestamp.Timestamp
}

// Transaction represents a transaction
type Transaction struct {
	db *DB
	// readOnly default value is true, once a write happen, then set to false
	readOnly bool
	// db       *DB
	// blockId starts at 0 and keeps incrementing,
	// this is used to name the file on s3 and then give it to tae to use
	blockId uint64
	// use for solving halloween problem
	statementId uint64
	meta        txn.TxnMeta
	// fileMaps used to store the mapping relationship between s3 filenames
	// and blockId
	fileMap map[string]uint64
	// writes cache stores any writes done by txn
	// every statement is an element
	writes   [][]Entry
	dnStores []DNStore
}

// Entry represents a delete/insert
type Entry struct {
	typ          int
	tableId      uint64
	databaseId   uint64
	tableName    string
	databaseName string
	// blockName for s3 file
	fileName string
	// blockId for s3 file
	blockId uint64
	// update or delete tuples
	bat     *batch.Batch
	dnStore DNStore
}

type database struct {
	databaseId   uint64
	databaseName string
	db           *DB
	m            *mheap.Mheap
	txn          *Transaction
}

type table struct {
	tableId   uint64
	tableName string
	db        *database
	defs      []engine.TableDef
}

type column struct {
	databaseId uint64
	// column name
	name            string
	tableName       string
	databaseName    string
	typ             []byte
	typLen          int32
	num             int32
	comment         string
	notNull         int8
	hasDef          int8
	defaultExpr     []byte
	constraintType  string
	isHidden        int8
	isAutoIncrement int8
}
