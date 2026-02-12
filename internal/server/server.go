package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"

	"minidb/internal/catalog"
	"minidb/internal/storage"
	"minidb/internal/txn"
	"minidb/pkg/types"
	// "minidb/internal/sql/parser" // We will call handleQuery logic which likely needs to be shared or Refactored.
	// Ideally main.go's handleQuery should be moved to a shared place?
	// Or Server calls a 'QueryHandler' interface.
)

type Server struct {
	engine  storage.StorageEngine
	catalog *catalog.Catalog
	tm      *txn.TxnManager
	addr    string
	// Handler function to execute SQL
	handler func(sql string, cat *catalog.Catalog, engine storage.StorageEngine, txID types.TxnID) string
}

func NewServer(addr string, engine storage.StorageEngine, cat *catalog.Catalog, tm *txn.TxnManager, handler func(string, *catalog.Catalog, storage.StorageEngine, types.TxnID) string) *Server {
	return &Server{
		addr:    addr,
		engine:  engine,
		catalog: cat,
		tm:      tm,
		handler: handler,
	}
}

func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	fmt.Printf("Server listening on %s\n", s.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Accept error: %v\n", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("New connection from %s\n", conn.RemoteAddr())

	// Create a transaction/session context if needed.
	// Supporting Txn across requests requires stateful connection.
	// For simplicity, AutoCommit for now, or maintain TxnID in this loop.

	var currentTx *txn.Transaction

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		// Read Line (Terminated by \n)
		cmd, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if cmd == "exit" || cmd == "quit" {
			return
		}

		// Txn Handling
		if strings.ToUpper(cmd) == "BEGIN" {
			if currentTx != nil {
				writer.WriteString("Error: Transaction already in progress\n")
			} else {
				currentTx = s.tm.Begin(context.Background())
				writer.WriteString(fmt.Sprintf("Txn %d started\n", currentTx.ID))
			}
			writer.Flush()
			continue
		}
		if strings.ToUpper(cmd) == "COMMIT" {
			if currentTx == nil {
				writer.WriteString("Error: No active transaction\n")
			} else {
				s.tm.Commit(context.Background(), currentTx)
				writer.WriteString(fmt.Sprintf("Txn %d committed\n", currentTx.ID))
				currentTx = nil
			}
			writer.Flush()
			continue
		}
		if strings.ToUpper(cmd) == "ROLLBACK" {
			if currentTx == nil {
				writer.WriteString("Error: No active transaction\n")
			} else {
				s.tm.Abort(context.Background(), currentTx)
				writer.WriteString(fmt.Sprintf("Txn %d aborted\n", currentTx.ID))
				currentTx = nil
			}
			writer.Flush()
			continue
		}

		// Execution
		isAutoCommit := false
		txToUse := currentTx
		if txToUse == nil {
			txToUse = s.tm.Begin(context.Background())
			isAutoCommit = true
		}

		// Execute
		// We need to capture output.
		// Refactoring handleQuery to return string or accept a writer is best.
		// For now, let's assume we pass a function that returns output string.
		output := s.handler(cmd, s.catalog, s.engine, txToUse.ID)
		writer.WriteString(output + "\n") // Add newline to end
		writer.Flush()

		if isAutoCommit {
			s.tm.Commit(context.Background(), txToUse)
		}
	}
}
