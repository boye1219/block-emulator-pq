// the define and some operation of txpool

package core

import (
	"blockEmulator/utils"
	"container/heap"
	"fmt"
	"math/big"
	"sync"
	"time"
	"unsafe"
)

type TxPriorityQueue []*Transaction

func (pq TxPriorityQueue) Len() int { return len(pq) }

func (pq TxPriorityQueue) Less(i, j int) bool { 
	if pq[i].GasPrice.Cmp(pq[j].GasPrice) != 0 {
		return pq[i].GasPrice.Cmp(pq[j].GasPrice) > 0
	}
	return pq[i].Time.Before(pq[j].Time)
}

func (pq TxPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *TxPriorityQueue) Push(x any) {
	tx := x.(*Transaction)
	*pq = append(*pq, tx)
}

func (pq *TxPriorityQueue) PushTx(tx *Transaction) {
	pq.Push(tx)
}

func (pq *TxPriorityQueue) Pop() any {
	old := *pq
	n := pq.Len()
	x := old[n-1]
	*pq = old[0 : n-1]
	return x
}

func (pq *TxPriorityQueue) PopTx() *Transaction {
	poppedTx := pq.Pop().(*Transaction)
	return poppedTx
}

type TxPool struct {
	TxQueue   *TxPriorityQueue          // transaction Queue
	RelayPool map[uint64][]*Transaction // designed for sharded blockchain, from Monoxide
	lock      sync.Mutex
	// The pending list is ignored
}

func NewTxPool() *TxPool {
	pq := make(TxPriorityQueue, 0)
	heap.Init(&pq)
	return &TxPool{
		TxQueue:   &pq,
		RelayPool: make(map[uint64][]*Transaction),
	}
}

// Add a transaction to the pool (consider the queue only)
func (txpool *TxPool) AddTx2Pool(tx *Transaction) {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	if tx.Time.IsZero() {
		tx.Time = time.Now()
	}
	heap.Push(txpool.TxQueue, tx)
}

// Add a list of transactions to the pool
func (txpool *TxPool) AddTxs2Pool(txs []*Transaction) {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	for _, tx := range txs {
		if tx.Time.IsZero() {
			tx.Time = time.Now()
		}
		heap.Push(txpool.TxQueue, tx)
	}
}

// Pack transactions for a proposal
func (txpool *TxPool) PackTxs(max_txs uint64) []*Transaction {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()

	txs_Packed := make([]*Transaction, 0)
	txNum := max_txs
	if uint64(txpool.TxQueue.Len()) < txNum {
		txNum = uint64(txpool.TxQueue.Len())
	}

	for i := uint64(0); i < txNum; i++ {
		if txpool.TxQueue.Len() > 0 {
			tx_popped := heap.Pop(txpool.TxQueue).(*Transaction)

			txs_Packed = append(txs_Packed, tx_popped)
		}
	}

    if len(txs_Packed) == 0 {
        fmt.Println("Packed 0 transactions, average GasPrice: 0")
        return txs_Packed
    }

    totalGasPrice := new(big.Int)
    for _, tx := range txs_Packed {
        if tx.GasPrice != nil {
            totalGasPrice.Add(totalGasPrice, tx.GasPrice)
        }
    }

    txCount := big.NewInt(int64(len(txs_Packed)))

    avgGasPrice := new(big.Int)
    avgGasPrice.Div(totalGasPrice, txCount)

    fmt.Printf("==================== Packed %d transactions, Average GasPrice: %s ====================\n",
				len(txs_Packed), avgGasPrice.String())

	return txs_Packed
}

// Pack transaction for a proposal (use 'BlocksizeInBytes' to control)
func (txpool *TxPool) PackTxsWithBytes(max_bytes int) []*Transaction {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()

	txs_Packed := make([]*Transaction, 0)
	currentSize := 0

	for txpool.TxQueue.Len() > 0 {
		tx := heap.Pop(txpool.TxQueue).(*Transaction)
		currentSize += int(unsafe.Sizeof(*tx))
		txs_Packed = append(txs_Packed, tx)
		if currentSize > max_bytes {
			break
		}
	}
	return txs_Packed
}

// Relay transactions
func (txpool *TxPool) AddRelayTx(tx *Transaction, shardID uint64) {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	_, ok := txpool.RelayPool[shardID]
	if !ok {
		txpool.RelayPool[shardID] = make([]*Transaction, 0)
	}
	txpool.RelayPool[shardID] = append(txpool.RelayPool[shardID], tx)
}

// txpool get locked
func (txpool *TxPool) GetLocked() {
	txpool.lock.Lock()
}

// txpool get unlocked
func (txpool *TxPool) GetUnlocked() {
	txpool.lock.Unlock()
}

// get the length of tx queue
func (txpool *TxPool) GetTxQueueLen() int {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	return txpool.TxQueue.Len()
}

// get the length of ClearRelayPool
func (txpool *TxPool) ClearRelayPool() {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	txpool.RelayPool = nil
}

// abort ! Pack relay transactions from relay pool
func (txpool *TxPool) PackRelayTxs(shardID, minRelaySize, maxRelaySize uint64) ([]*Transaction, bool) {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()
	if _, ok := txpool.RelayPool[shardID]; !ok {
		return nil, false
	}
	if len(txpool.RelayPool[shardID]) < int(minRelaySize) {
		return nil, false
	}
	txNum := maxRelaySize
	if uint64(len(txpool.RelayPool[shardID])) < txNum {
		txNum = uint64(len(txpool.RelayPool[shardID]))
	}
	relayTxPacked := txpool.RelayPool[shardID][:txNum]
	txpool.RelayPool[shardID] = txpool.RelayPool[shardID][txNum:]
	return relayTxPacked, true
}

// abort ! Transfer transactions when re-sharding
func (txpool *TxPool) TransferTxs(addr utils.Address) []*Transaction {
	txpool.lock.Lock()
	defer txpool.lock.Unlock()

	txTransfered := make([]*Transaction, 0)
	newTxHeap := make(TxPriorityQueue, 0)

	// Pop all transactions from the heap
	tempTxs := make([]*Transaction, 0)
	for txpool.TxQueue.Len() > 0 {
		tempTxs = append(tempTxs, heap.Pop(txpool.TxQueue).(*Transaction))
	}

	// Separate transactions by sender
	for _, tx := range tempTxs {
		if tx.Sender == addr {
			txTransfered = append(txTransfered, tx)
		} else {
			newTxHeap = append(newTxHeap, tx)
		}
	}

	// Rebuild the heap with remaining transactions
	heap.Init(&newTxHeap)
	txpool.TxQueue = &newTxHeap

	// Process relay pool
	newRelayPool := make(map[uint64][]*Transaction)
	for shardID, shardPool := range txpool.RelayPool {
		for _, tx := range shardPool {
			if tx.Sender == addr {
				txTransfered = append(txTransfered, tx)
			} else {
				if _, ok := newRelayPool[shardID]; !ok {
					newRelayPool[shardID] = make([]*Transaction, 0)
				}
				newRelayPool[shardID] = append(newRelayPool[shardID], tx)
			}
		}
	}
	txpool.RelayPool = newRelayPool
	return txTransfered
}
