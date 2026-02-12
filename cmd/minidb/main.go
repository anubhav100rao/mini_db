package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"minidb/internal/catalog"
	"minidb/internal/executor"
	"minidb/internal/server"
	"minidb/internal/sql/binder"
	"minidb/internal/sql/parser"
	"minidb/internal/sql/plan"
	"minidb/internal/storage"
	"minidb/internal/txn"
	"minidb/internal/wal"
	"minidb/pkg/types"
)

func main() {
	serverMode := flag.Bool("server", false, "Run in server mode")
	port := flag.String("port", "3000", "Port to listen on")
	flag.Parse()

	fmt.Println("MiniDB - The Go Database Engine")

	// 1. Initialize Components
	os.MkdirAll("data", 0755)
	walLog, err := wal.NewWAL("data/wal.log")
	if err != nil {
		panic(err)
	}

	tm := txn.NewTxnManager()
	engine := storage.NewLSMEngine("data", walLog, tm)
	defer engine.Close()

	cat := catalog.NewCatalog()
	schema := types.Schema{
		ColNames: []string{"id", "name"},
		ColTypes: []string{"int", "string"},
	}
	tid, _ := cat.CreateTable("users", schema) // Returns ID 1

	// Create Index on "id" (col 0)
	idxName := "idx_users_id"
	engine.CreateSecondaryIndex(idxName)
	engine.RegisterIndex(tid, 0, idxName) // 0 is 'id' column
	cat.CreateIndex("users", "id", idxName)

	fmt.Println("System: Created Hash Index on users(id)")

	if *serverMode {
		addr := ":" + *port
		srv := server.NewServer(addr, engine, cat, tm, ExecuteQuery)
		if err := srv.Run(context.Background()); err != nil {
			panic(err)
		}
		return
	}

	// 2. REPL Loop
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Running in REPL mode. Use -server to start TCP server.")
	var currentTx *txn.Transaction // Current session transaction

	for {
		fmt.Print("minidb> ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		if text == "exit" || text == "quit" {
			break
		}
		if text == "" {
			continue
		}

		// Handle Transaction Commands
		if strings.ToUpper(text) == "BEGIN" {
			if currentTx != nil {
				fmt.Println("Transaction already in progress")
			} else {
				currentTx = tm.Begin(context.Background())
				fmt.Printf("Txn %d started\n", currentTx.ID)
			}
			continue
		}
		if strings.ToUpper(text) == "COMMIT" {
			if currentTx == nil {
				fmt.Println("No active transaction")
			} else {
				tm.Commit(context.Background(), currentTx)
				fmt.Printf("Txn %d committed\n", currentTx.ID)
				currentTx = nil
			}
			continue
		}
		if strings.ToUpper(text) == "ROLLBACK" || strings.ToUpper(text) == "ABORT" {
			if currentTx == nil {
				fmt.Println("No active transaction")
			} else {
				tm.Abort(context.Background(), currentTx)
				fmt.Printf("Txn %d aborted\n", currentTx.ID)
				currentTx = nil
			}
			continue
		}

		// Maintenance Commands
		if strings.ToUpper(text) == "FLUSH" {
			if err := engine.Flush(context.Background()); err != nil {
				fmt.Printf("Flush Error: %v\n", err)
			} else {
				fmt.Println("MemTable flushed to SSTable")
			}
			continue
		}
		if strings.ToUpper(text) == "COMPACT" {
			if err := engine.Compact(context.Background()); err != nil {
				fmt.Printf("Compact Error: %v\n", err)
			} else {
				fmt.Println("Compaction completed")
			}
			continue
		}

		// Auto-commit
		isAutoCommit := false
		txToUse := currentTx
		if txToUse == nil {
			txToUse = tm.Begin(context.Background())
			isAutoCommit = true
		}

		handleQuery(text, cat, engine, txToUse.ID)

		if isAutoCommit {
			tm.Commit(context.Background(), txToUse)
		}
	}
}

func handleQuery(sql string, cat *catalog.Catalog, engine storage.StorageEngine, txID types.TxnID) {
	fmt.Print(ExecuteQuery(sql, cat, engine, txID))
}

func ExecuteQuery(sql string, cat *catalog.Catalog, engine storage.StorageEngine, txID types.TxnID) string {
	var sb strings.Builder

	// Metadata Commands
	upperSQL := strings.ToUpper(strings.TrimSpace(sql))
	if upperSQL == "HELP" {
		return `Available Commands:
  HELP                  Show this help
  SHOW TABLES           List all tables
  SHOW DATABASES        List all databases
  USE <database>        Switch database
  DESCRIBE <table_name> Show table schema
  
  SQL Support:
  SELECT * FROM <table> [WHERE <col> = <val>]
  INSERT INTO <table> VALUES (<val>, ...), ...
  BEGIN / COMMIT / ROLLBACK

  Maintenance:
  FLUSH                 Flush MemTable to disk
  COMPACT               Run compaction
`
	}
	if upperSQL == "SHOW TABLES" || upperSQL == "SHOW TABLES;" {
		names := cat.GetTableNames()
		return fmt.Sprintf("Tables:\n%s\n(%d rows)\n", strings.Join(names, "\n"), len(names))
	}
	if upperSQL == "SHOW DATABASES" || upperSQL == "SHOW DATABASES;" {
		return "Databases:\nminidb\n(1 rows)\n"
	}
	if strings.HasPrefix(upperSQL, "USE ") {
		parts := strings.Fields(upperSQL)
		if len(parts) < 2 {
			return "Usage: USE <database_name>\n"
		}
		dbName := parts[1]
		dbName = strings.TrimSuffix(dbName, ";")

		if dbName == "MINIDB" {
			return "Database changed\n"
		}
		return fmt.Sprintf("Error: Unknown database '%s'\n", dbName)
	}
	if strings.HasPrefix(upperSQL, "DESCRIBE ") {
		parts := strings.Fields(sql) // Use original SQL for arguments
		if len(parts) < 2 {
			return "Usage: DESCRIBE <table_name>\n"
		}
		tableName := parts[1]
		// Remove trailing semicolon if present
		tableName = strings.TrimSuffix(tableName, ";")

		tableInfo, err := cat.GetTableByName(tableName)
		if err != nil {
			return fmt.Sprintf("Error: %v\n", err)
		}

		var schemaSb strings.Builder
		schemaSb.WriteString(fmt.Sprintf("Table: %s\n", tableName))
		schemaSb.WriteString("Column | Type\n")
		schemaSb.WriteString("-------+-----\n")
		for i, col := range tableInfo.Schema.ColNames {
			schemaSb.WriteString(fmt.Sprintf("%-6s | %s\n", col, tableInfo.Schema.ColTypes[i]))
		}
		return schemaSb.String()
	}

	// Maintenance Commands
	if upperSQL == "FLUSH" {
		if err := engine.Flush(context.Background()); err != nil {
			return fmt.Sprintf("Flush Error: %v\n", err)
		}
		return "MemTable flushed to SSTable\n"
	}
	if upperSQL == "COMPACT" {
		if err := engine.Compact(context.Background()); err != nil {
			return fmt.Sprintf("Compact Error: %v\n", err)
		}
		return "Compaction completed\n"
	}

	// A. Parse
	stmt, err := parser.Parse(sql)
	if err != nil {
		return fmt.Sprintf("Parse Error: %v\n", err)
	}

	// B. Bind
	b := binder.NewBinder(cat)
	logicalPlan, err := b.Bind(stmt)
	if err != nil {
		return fmt.Sprintf("Bind Error: %v\n", err)
	}

	// C. Build Executor
	ctx := &executor.ExecContext{
		Storage: engine,
		TxnID:   txID,
	}

	var exec executor.Executor
	switch p := logicalPlan.(type) {
	case *plan.SeqScanNode:
		// sb.WriteString("Explain: SeqScan\n")
		exec = executor.NewSeqScan(ctx, p)
	case *plan.InsertNode:
		// sb.WriteString("Explain: Insert\n")
		exec = executor.NewInsertExec(ctx, p)
	case *plan.FilterNode:
		sb.WriteString("Explain: Filter->SeqScan\n")
		childNode, ok := p.Child.(*plan.SeqScanNode)
		if !ok {
			return fmt.Sprintf("Execution Error: Filter child must be SeqScan for now\n")
		}
		childExec := executor.NewSeqScan(ctx, childNode)
		exec = executor.NewFilterExec(ctx, p, childExec)
	case *plan.IndexScanNode:
		sb.WriteString(fmt.Sprintf("Explain: IndexScan(index=%s)\n", p.IndexName))
		idx := engine.GetIndex(p.IndexName)
		if idx == nil {
			return fmt.Sprintf("Execution Error: Index %s not found in engine\n", p.IndexName)
		}
		exec = executor.NewIndexScan(ctx, p, idx)
	default:
		return fmt.Sprintf("Execution Error: unsupported plan type %T\n", p)
	}

	// D. Run
	if err := exec.Open(context.Background()); err != nil {
		return fmt.Sprintf("Runtime Error (Open): %v\n", err)
	}
	defer exec.Close(context.Background())

	count := 0
	for {
		tuple, err := exec.Next(context.Background())
		if tuple == nil {
			break // Done or EOF
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Sprintf("Runtime Error (Next): %v\n", err)
		}

		sb.WriteString(fmt.Sprintf("%v\n", tuple.Values))
		count++
	}
	sb.WriteString(fmt.Sprintf("(%d rows)\n", count))
	return sb.String()
}
