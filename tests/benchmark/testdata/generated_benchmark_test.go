//go:build margo_generated_benchmark

package MargoTest_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	gormysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"

	generated "github.com/rah-0/margo/tests/benchmark/generatedtest/MargoTest"
	margoalpha "github.com/rah-0/margo/tests/benchmark/generatedtest/MargoTest/Alpha"
	"github.com/rah-0/margo/tests/benchmark/generatedtest/ent"
	entalpha "github.com/rah-0/margo/tests/benchmark/generatedtest/ent/alpha"
)

const (
	insertAlphaSQL = "INSERT INTO `alpha` (`Uuid`, `FirstInsert`, `LastUpdate`, `Animal`, `BigNumber`, `test_field`) VALUES (?, ?, ?, ?, ?, ?)"
	deleteAlphaSQL = "DELETE FROM `alpha` WHERE `Uuid` = ?"
	selectAlphaSQL = "SELECT `Uuid`, `FirstInsert`, `LastUpdate`, `Animal`, `BigNumber`, `test_field` FROM `alpha` WHERE `Uuid` = ?"

	benchmarkTimestamp = "2024-01-01 15:04:05.000000"
	benchmarkAnimal    = "Animal"
	benchmarkBigNumber = "1234567890"
	benchmarkTestField = "Test"
)

var (
	benchmarkDB        *sql.DB
	benchmarkBunDB     *bun.DB
	benchmarkGORMDB    *gorm.DB
	benchmarkEntClient *ent.Client
	benchmarkUUIDSink  string
)

type alphaData struct {
	UUID        string
	FirstInsert string
	LastUpdate  string
	Animal      string
	BigNumber   string
	TestField   string
}

type bunAlpha struct {
	bun.BaseModel `bun:"table:alpha"`
	UUID          string `bun:"Uuid,pk"`
	FirstInsert   string `bun:"FirstInsert"`
	LastUpdate    string `bun:"LastUpdate"`
	Animal        string `bun:"Animal"`
	BigNumber     string `bun:"BigNumber"`
	TestField     string `bun:"test_field"`
}

type gormAlpha struct {
	UUID        string `gorm:"column:Uuid;primaryKey"`
	FirstInsert string `gorm:"column:FirstInsert"`
	LastUpdate  string `gorm:"column:LastUpdate"`
	Animal      string `gorm:"column:Animal"`
	BigNumber   string `gorm:"column:BigNumber"`
	TestField   string `gorm:"column:test_field"`
}

func (gormAlpha) TableName() string {
	return "alpha"
}

func TestMain(m *testing.M) {
	os.Exit(runBenchmarks(m))
}

func runBenchmarks(m *testing.M) int {
	dsn := os.Getenv("MARGO_BENCHMARK_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "MARGO_BENCHMARK_DSN is required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	benchmarkDB, err = sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open benchmark database: %v\n", err)
		return 2
	}
	if err := benchmarkDB.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping benchmark database: %v\n", err)
		_ = benchmarkDB.Close()
		return 2
	}
	if err := generated.SetDB(benchmarkDB); err != nil {
		fmt.Fprintf(os.Stderr, "initialize generated MarGO packages: %v\n", err)
		_ = benchmarkDB.Close()
		return 2
	}

	benchmarkBunDB = bun.NewDB(benchmarkDB, mysqldialect.New())
	benchmarkGORMDB, err = gorm.Open(gormysql.New(gormysql.Config{
		Conn:                      benchmarkDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize GORM: %v\n", err)
		_ = benchmarkDB.Close()
		return 2
	}

	benchmarkEntClient, err = ent.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize Ent: %v\n", err)
		_ = benchmarkDB.Close()
		return 2
	}

	exitCode := m.Run()
	if err := benchmarkEntClient.Close(); err != nil && exitCode == 0 {
		fmt.Fprintf(os.Stderr, "close Ent benchmark client: %v\n", err)
		exitCode = 1
	}
	if err := benchmarkDB.Close(); err != nil && exitCode == 0 {
		fmt.Fprintf(os.Stderr, "close benchmark database: %v\n", err)
		exitCode = 1
	}
	return exitCode
}

func BenchmarkEntityDBInsertRawSQL(b *testing.B) {
	prepareBenchmark(b)
	ctx := b.Context()
	statement := prepareStatement(b, insertAlphaSQL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := newAlphaData()
		result, err := statement.ExecContext(
			ctx,
			row.UUID,
			row.FirstInsert,
			row.LastUpdate,
			row.Animal,
			row.BigNumber,
			row.TestField,
		)
		if err != nil {
			b.Fatal(err)
		}
		requireOneRow(b, result)
		benchmarkUUIDSink = row.UUID
	}
}

func BenchmarkEntityDBInsertMarGO(b *testing.B) {
	prepareBenchmark(b)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := newAlphaData()
		entity := margoEntity(row)
		result := entity.DBInsertCtx(ctx, margoalpha.NewQueryParams().WithInsert(margoalpha.Fields...))
		if result.Error != nil {
			b.Fatal(result.Error)
		}
		requireOneRow(b, result.Result)
		benchmarkUUIDSink = entity.Uuid
	}
}

func BenchmarkEntityDBInsertBun(b *testing.B) {
	prepareBenchmark(b)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := newAlphaData()
		entity := bunEntity(row)
		result, err := benchmarkBunDB.NewInsert().Model(&entity).Exec(ctx)
		if err != nil {
			b.Fatal(err)
		}
		requireOneRow(b, result)
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBInsertGorm(b *testing.B) {
	prepareBenchmark(b)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := newAlphaData()
		entity := gormEntity(row)
		result := benchmarkGORMDB.WithContext(ctx).Create(&entity)
		if result.Error != nil {
			b.Fatal(result.Error)
		}
		if result.RowsAffected != 1 {
			b.Fatalf("insert affected %d rows, expected 1", result.RowsAffected)
		}
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBInsertEnt(b *testing.B) {
	prepareBenchmark(b)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := newAlphaData()
		entity, err := benchmarkEntClient.Alpha.Create().
			SetID(row.UUID).
			SetFirstInsert(row.FirstInsert).
			SetLastUpdate(row.LastUpdate).
			SetAnimal(row.Animal).
			SetBigNumber(row.BigNumber).
			SetTestField(row.TestField).
			Save(ctx)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkUUIDSink = entity.ID
	}
}

func BenchmarkEntityDBDeleteRawSQL(b *testing.B) {
	prepareBenchmark(b)
	rows := seedAlphaRows(b, b.N)
	ctx := b.Context()
	statement := prepareStatement(b, deleteAlphaSQL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := statement.ExecContext(ctx, rows[i].UUID)
		if err != nil {
			b.Fatal(err)
		}
		requireOneRow(b, result)
		benchmarkUUIDSink = rows[i].UUID
	}
}

func BenchmarkEntityDBDeleteMarGO(b *testing.B) {
	prepareBenchmark(b)
	rows := seedAlphaRows(b, b.N)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := margoalpha.Entity{Uuid: rows[i].UUID}
		result := entity.DBDeleteCtx(ctx, margoalpha.NewQueryParams().WithWhere(margoalpha.FieldUuid))
		if result.Error != nil {
			b.Fatal(result.Error)
		}
		requireOneRow(b, result.Result)
		benchmarkUUIDSink = entity.Uuid
	}
}

func BenchmarkEntityDBDeleteBun(b *testing.B) {
	prepareBenchmark(b)
	rows := seedAlphaRows(b, b.N)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := bunAlpha{UUID: rows[i].UUID}
		result, err := benchmarkBunDB.NewDelete().
			Model(&entity).
			Where("`Uuid` = ?", entity.UUID).
			Exec(ctx)
		if err != nil {
			b.Fatal(err)
		}
		requireOneRow(b, result)
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBDeleteGorm(b *testing.B) {
	prepareBenchmark(b)
	rows := seedAlphaRows(b, b.N)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := gormAlpha{UUID: rows[i].UUID}
		result := benchmarkGORMDB.WithContext(ctx).Delete(&entity)
		if result.Error != nil {
			b.Fatal(result.Error)
		}
		if result.RowsAffected != 1 {
			b.Fatalf("delete affected %d rows, expected 1", result.RowsAffected)
		}
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBDeleteEnt(b *testing.B) {
	prepareBenchmark(b)
	rows := seedAlphaRows(b, b.N)
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count, err := benchmarkEntClient.Alpha.Delete().
			Where(entalpha.IDEQ(rows[i].UUID)).
			Exec(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if count != 1 {
			b.Fatalf("delete affected %d rows, expected 1", count)
		}
		benchmarkUUIDSink = rows[i].UUID
	}
}

func BenchmarkEntityDBSelectRawSQL(b *testing.B) {
	prepareBenchmark(b)
	want := seedAlphaRows(b, 1)[0]
	ctx := b.Context()
	statement := prepareStatement(b, selectAlphaSQL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var entity alphaData
		if err := statement.QueryRowContext(ctx, want.UUID).Scan(
			&entity.UUID,
			&entity.FirstInsert,
			&entity.LastUpdate,
			&entity.Animal,
			&entity.BigNumber,
			&entity.TestField,
		); err != nil {
			b.Fatal(err)
		}
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBSelectMarGO(b *testing.B) {
	prepareBenchmark(b)
	want := seedAlphaRows(b, 1)[0]
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := margoalpha.Entity{Uuid: want.UUID}
		result := entity.DBSelectCtx(ctx, margoalpha.NewQueryParams().
			WithSelect(margoalpha.Fields...).
			WithWhere(margoalpha.FieldUuid))
		if result.Error != nil {
			b.Fatal(result.Error)
		}
		if len(result.Entities) != 1 {
			b.Fatalf("select returned %d rows, expected 1", len(result.Entities))
		}
		benchmarkUUIDSink = result.Entities[0].Uuid
	}
}

func BenchmarkEntityDBSelectBun(b *testing.B) {
	prepareBenchmark(b)
	want := seedAlphaRows(b, 1)[0]
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var entity bunAlpha
		if err := benchmarkBunDB.NewSelect().
			Model(&entity).
			Where("`Uuid` = ?", want.UUID).
			Limit(1).
			Scan(ctx); err != nil {
			b.Fatal(err)
		}
		benchmarkUUIDSink = entity.UUID
	}
}

func BenchmarkEntityDBSelectGorm(b *testing.B) {
	prepareBenchmark(b)
	want := seedAlphaRows(b, 1)[0]
	ctx := b.Context()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var entity gormAlpha
		if err := benchmarkGORMDB.WithContext(ctx).
			Where("`Uuid` = ?", want.UUID).
			First(&entity).Error; err != nil {
			b.Fatal(err)
		}
		benchmarkUUIDSink = entity.UUID
	}
}

func prepareBenchmark(b *testing.B) {
	b.Helper()
	b.ReportAllocs()

	if _, err := benchmarkDB.ExecContext(b.Context(), "TRUNCATE TABLE `alpha`"); err != nil {
		b.Fatalf("truncate alpha before benchmark: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := benchmarkDB.ExecContext(ctx, "TRUNCATE TABLE `alpha`"); err != nil {
			b.Errorf("truncate alpha after benchmark: %v", err)
		}
	})
}

func prepareStatement(b *testing.B, query string) *sql.Stmt {
	b.Helper()

	statement, err := benchmarkDB.PrepareContext(b.Context(), query)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := statement.Close(); err != nil {
			b.Errorf("close prepared statement: %v", err)
		}
	})
	return statement
}

func seedAlphaRows(b *testing.B, count int) []alphaData {
	b.Helper()

	statement := prepareStatement(b, insertAlphaSQL)
	rows := make([]alphaData, count)
	for i := range rows {
		row := newAlphaData()
		if _, err := statement.ExecContext(
			b.Context(),
			row.UUID,
			row.FirstInsert,
			row.LastUpdate,
			row.Animal,
			row.BigNumber,
			row.TestField,
		); err != nil {
			b.Fatalf("seed alpha row %d: %v", i, err)
		}
		rows[i] = row
	}
	return rows
}

func newAlphaData() alphaData {
	return alphaData{
		UUID:        uuid.NewString(),
		FirstInsert: benchmarkTimestamp,
		LastUpdate:  benchmarkTimestamp,
		Animal:      benchmarkAnimal,
		BigNumber:   benchmarkBigNumber,
		TestField:   benchmarkTestField,
	}
}

func margoEntity(row alphaData) margoalpha.Entity {
	return margoalpha.Entity{
		Uuid:        row.UUID,
		FirstInsert: row.FirstInsert,
		LastUpdate:  row.LastUpdate,
		Animal:      row.Animal,
		BigNumber:   row.BigNumber,
		TestField:   row.TestField,
	}
}

func bunEntity(row alphaData) bunAlpha {
	return bunAlpha{
		UUID:        row.UUID,
		FirstInsert: row.FirstInsert,
		LastUpdate:  row.LastUpdate,
		Animal:      row.Animal,
		BigNumber:   row.BigNumber,
		TestField:   row.TestField,
	}
}

func gormEntity(row alphaData) gormAlpha {
	return gormAlpha{
		UUID:        row.UUID,
		FirstInsert: row.FirstInsert,
		LastUpdate:  row.LastUpdate,
		Animal:      row.Animal,
		BigNumber:   row.BigNumber,
		TestField:   row.TestField,
	}
}

func requireOneRow(b *testing.B, result sql.Result) {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		b.Fatal(err)
	}
	if rowsAffected != 1 {
		b.Fatalf("operation affected %d rows, expected 1", rowsAffected)
	}
}
