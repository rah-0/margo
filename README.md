![MarGO logo](https://github.com/rah-0/margo-test/blob/master/margo.png "MariaDB's Sea Lion with Golang's Gopher")

[![Go Report Card](https://goreportcard.com/badge/github.com/rah-0/margo?v=1)](https://goreportcard.com/report/github.com/rah-0/margo)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

<a href="https://www.buymeacoffee.com/rah.0" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/arial-orange.png" alt="Buy Me A Coffee" height="50"></a>

# MarGO: A Simple, Reflection-free ORM for MariaDB and Go

MarGO (MariaDB + GO) is a code generator that creates type-safe database interaction code, mapping MariaDB table schemas to Go structs with zero reflection. This approach provides near-raw SQL performance while maintaining some ORM convenience.

> **Compatibility Note**: MarGO has been tested with MariaDB but not MySQL. Since both use the same underlying driver (`github.com/go-sql-driver/mysql`), it should be compatible with MySQL as well, but this has not been explicitly tested.

## Features

- **Reflection-free Design**: Unlike most ORMs, MarGO doesn't use reflection at runtime, eliminating that performance cost
- **Code Generation**: Automatically generates Go code from your database schema
- **Prepared Statement Caching**: Improves performance by reusing prepared statements
- **Direct Table Mapping**: Maps database tables directly to Go structs without complex abstractions
- **Context Support**: All database operations have context-aware versions
- **Strongly Typed**: Generated code is type-safe
- **Near-Raw SQL Performance**: Benchmarks show performance within 99-105% of raw SQL
- **Minimal Dependencies**: Lightweight with few external dependencies
- **No Nil Pointer Errors**: Safely handles NULL database fields by sanitizing them to empty strings, preventing the common nil pointer errors that occur in other ORMs

## Getting Started

### Installation

Install the MarGO CLI tool:

```bash
go install github.com/rah-0/margo@latest
```

### CLI Usage

MarGO provides a command-line interface to generate code from your database schema:

```bash
margo -dbUser="your_db_user" \
      -dbPassword="your_db_password" \
      -dbName="your_database" \
      -dbIp="localhost" \
      -dbPort="3306" \
      -outputPath="/path/must/be/directory" \
      -queriesPath="/path/must/be/file.sql"
```

### CLI Parameters

| Parameter     | Description                                   | Default | Required |
|---------------|-----------------------------------------------|---------|----------|
| `-dbUser`     | Database username                             | -       | Yes      |
| `-dbPassword` | Database password                             | -       | Yes      |
| `-dbName`     | Database name                                 | -       | Yes      |
| `-dbIp`       | Database IP address                           | -       | Yes      |
| `-dbPort`     | Database port                                 | 3306    | Yes      |
| `-outputPath` | Directory where generated files will be saved | -       | Yes      |
| `-queriesPath`| Optional path for SQL queries file            | -       | No       |

## Custom SQL Queries

MarGO allows you to define custom SQL queries that will be transformed into type-safe Go functions:

### Requirements

- All custom queries must be in a single file (specified by `-queriesPath`)
- Each query must be prefixed with `-- Name: FunctionName` which will be the name of the generated Go function
- Queries must end with a semicolon (`;`) and be separated by at least one newline
- No `SELECT *` queries are allowed, you must explicitly specify columns

### Example

```sql
-- Name: GetUsersByRole
SELECT id, username, email FROM users WHERE role = ?;

-- Name: CountActiveUsers
SELECT COUNT(*) as count FROM users WHERE status = 'active';
```

### Using Generated Query Functions

Before using any generated code (tables or custom queries), you must set the database connection once:

```go
import (
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
    "yourProject/yourDatabase"
)

func main() {
    db, err := sql.Open("mysql", "connection_string")
    if err != nil {
        // Handle error
    }
    
    // Set the database connection for the generated packages
    yourDatabase.SetDB(db)
    yourDatabaseEntityA.SetDB(db)
    yourDatabaseEntityB.SetDB(db)
    
    // Now you can use your custom query
    users, err := yourDatabase.GetUsersByRole("admin")
    // ...
}
```

## How It Works

MarGO works differently from traditional ORMs:

1. It connects to your MariaDB database and reads the schema information
2. For each table, it generates a Go file with:
   - A struct representing the table
   - Constants for field names
   - Type-safe CRUD functions
   - Prepared statement caching
3. The generated code uses standard `database/sql` operations without reflection

## Generated Code

You can see examples of generated code in the [margo-test repository](https://github.com/rah-0/margo-test/tree/master/dbs/Template).

For examples of how to use the generated code, see these test files:
- [Entity usage examples](https://github.com/rah-0/margo-test/blob/master/dbs/Template/Alpha/entity_test.go)
- [Custom queries usage examples](https://github.com/rah-0/margo-test/blob/master/dbs/Template/queries_test.go)

## Performance

See the detailed benchmark results in the [BENCHMARKS.md](https://github.com/rah-0/margo-test/blob/master/BENCHMARKS.md) file in the margo-test repository.

---

[![Buy Me A Coffee](https://cdn.buymeacoffee.com/buttons/default-orange.png)](https://www.buymeacoffee.com/rah.0)
