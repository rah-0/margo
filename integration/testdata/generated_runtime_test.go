//go:build margo_generated_runtime

package MargoTest_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	generated "github.com/rah-0/margo/integration/generatedtest/MargoTest"
	"github.com/rah-0/margo/integration/generatedtest/MargoTest/AllTypes"
	"github.com/rah-0/margo/integration/generatedtest/MargoTest/Alpha"
	"github.com/rah-0/margo/integration/generatedtest/MargoTest/Beta"
)

var (
	database       *sql.DB
	identifierSeed atomic.Uint64
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("MARGO_INTEGRATION_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "MARGO_INTEGRATION_DSN is required")
		os.Exit(2)
	}

	var err error
	database, err = sql.Open("mysql", dsn)
	if err == nil {
		err = database.Ping()
	}
	if err == nil {
		err = generated.SetDB(database)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize generated runtime tests: %v\n", err)
		os.Exit(2)
	}

	exitCode := m.Run()
	if err := database.Close(); err != nil && exitCode == 0 {
		fmt.Fprintf(os.Stderr, "close generated runtime database: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

func TestEntityDBInsertWithUuid(t *testing.T) {
	resetTables(t)

	e := Alpha.Entity{
		Uuid:   testUUID(),
		Animal: "Cat",
	}

	result := e.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.FieldUuid, Alpha.FieldAnimal))
	if result.Error != nil {
		t.Fatal(result.Error)
	}
}

func TestEntityDBInsertWithUuidAndDelete(t *testing.T) {
	resetTables(t)

	e := Alpha.Entity{
		Uuid:   testUUID(),
		Animal: "Dog",
	}

	result := e.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.FieldUuid, Alpha.FieldAnimal))
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	result = e.DBDelete(Alpha.NewQueryParams().WithWhere(Alpha.FieldUuid))
	if result.Error != nil {
		t.Fatal(result.Error)
	}
}

func TestEntityDBSelectAll(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	entity := Alpha.Entity{
		Uuid:   uuid,
		Animal: "Fox",
	}

	result := entity.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.FieldUuid, Alpha.FieldAnimal))
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	result = Alpha.DBSelectAll()
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	for _, entity := range result.Entities {
		if entity.Uuid == uuid && entity.Animal == "Fox" {
			return
		}
	}
	t.Fatal("inserted entity not found in DBSelectAll results")
}

func TestEntityLastUpdateManualOverride(t *testing.T) {
	resetTables(t)

	e := Beta.Entity{
		Uuid: testUUID(),
		Name: "manual-update-test",
	}

	result := e.DBInsert(Beta.NewQueryParams().WithInsert(Beta.FieldUuid, Beta.FieldName))
	if result.Error != nil {
		t.Fatal("insert failed:", result.Error)
	}

	const expected = "2000-01-01 00:00:00.123456"
	e.LastUpdate = expected

	result = e.DBUpdate(Beta.NewQueryParams().WithUpdate(Beta.FieldLastUpdate).WithWhere(Beta.FieldUuid))
	if result.Error != nil {
		t.Fatal("update failed:", result.Error)
	}

	check := Beta.Entity{Uuid: e.Uuid}
	result = check.DBExists(Beta.NewQueryParams().WithWhere(Beta.FieldUuid))
	if result.Error != nil {
		t.Fatal("DBExists failed:", result.Error)
	}
	if !result.Exists {
		t.Fatal("entity not found after update")
	}
	if check.LastUpdate != expected {
		t.Fatalf("expected last_update %s, got %s", expected, check.LastUpdate)
	}
}

func TestAllFieldsRoundtrip(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	e := AllTypes.Entity{
		Id:              strconv.FormatUint(identifierSeed.Add(1), 10),
		TinySigned:      "42",
		TinyUnsigned:    "42",
		SmallSigned:     "42",
		SmallUnsigned:   "42",
		MediumSigned:    "42",
		MediumUnsigned:  "42",
		IntSigned:       "42",
		IntUnsigned:     "42",
		BigSigned:       "42",
		BigUnsigned:     "42",
		FloatField:      "1.23",
		DoubleField:     "3.14159",
		RealField:       "2.71828",
		DecimalField:    "1234567890.1234567890",
		DecField:        "12345.12345",
		NumericField:    "999.9999999",
		FixedField:      "9999.999999",
		Bit1:            "\x01",
		Bit8:            "\x7F",
		Bit64:           "\x00\x00\x00\x00\x00\x00\x00\x01",
		BoolField:       "1",
		BooleanField:    "0",
		CharField:       "char10___",
		VarcharField:    "varchar test",
		TextField:       "some long text",
		TinytextField:   "tinytext",
		MediumtextField: "mediumtext content",
		LongtextField:   "longtext content",
		EnumField:       "two",
		SetField:        "a,b",
		BinaryField:     string(append([]byte{0x01, 0x02, 0x03}, make([]byte, 13)...)),
		VarbinaryField:  string([]byte{0x04, 0x05, 0x06}),
		BlobField:       "blob_data",
		TinyblobField:   "tinyblob",
		MediumblobField: string(bytes.Repeat([]byte("M"), 128)),
		LongblobField:   string(bytes.Repeat([]byte("L"), 256)),
		DateField:       "2025-06-29",
		TimeField:       "12:34:56",
		YearField:       "2025",
		DatetimeField:   "2025-06-29 12:34:56.000000",
		TimestampField:  "2025-06-29 12:34:56",
		UuidField:       uuid,
	}

	result := e.DBInsert(AllTypes.NewQueryParams().WithInsert(AllTypes.Fields...))
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	result = AllTypes.DBSelectAll()
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	var found *AllTypes.Entity
	for _, entity := range result.Entities {
		if entity.UuidField == uuid {
			found = entity
			break
		}
	}
	if found == nil {
		t.Fatal("inserted entity not found")
	}

	assertField(t, "Id", found.Id, e.Id)
	assertField(t, "TinySigned", found.TinySigned, e.TinySigned)
	assertField(t, "TinyUnsigned", found.TinyUnsigned, e.TinyUnsigned)
	assertField(t, "SmallSigned", found.SmallSigned, e.SmallSigned)
	assertField(t, "SmallUnsigned", found.SmallUnsigned, e.SmallUnsigned)
	assertField(t, "MediumSigned", found.MediumSigned, e.MediumSigned)
	assertField(t, "MediumUnsigned", found.MediumUnsigned, e.MediumUnsigned)
	assertField(t, "IntSigned", found.IntSigned, e.IntSigned)
	assertField(t, "IntUnsigned", found.IntUnsigned, e.IntUnsigned)
	assertField(t, "BigSigned", found.BigSigned, e.BigSigned)
	assertField(t, "BigUnsigned", found.BigUnsigned, e.BigUnsigned)
	assertField(t, "FloatField", found.FloatField, e.FloatField)
	assertField(t, "DoubleField", found.DoubleField, e.DoubleField)
	assertField(t, "RealField", found.RealField, e.RealField)
	assertField(t, "DecimalField", found.DecimalField, e.DecimalField)
	assertField(t, "DecField", found.DecField, e.DecField)
	assertField(t, "NumericField", found.NumericField, e.NumericField)
	assertField(t, "FixedField", found.FixedField, e.FixedField)
	assertField(t, "Bit1", found.Bit1, e.Bit1)
	assertField(t, "Bit8", found.Bit8, e.Bit8)
	assertField(t, "Bit64", found.Bit64, e.Bit64)
	assertField(t, "BoolField", found.BoolField, e.BoolField)
	assertField(t, "BooleanField", found.BooleanField, e.BooleanField)
	assertField(t, "CharField", found.CharField, e.CharField)
	assertField(t, "VarcharField", found.VarcharField, e.VarcharField)
	assertField(t, "TextField", found.TextField, e.TextField)
	assertField(t, "TinytextField", found.TinytextField, e.TinytextField)
	assertField(t, "MediumtextField", found.MediumtextField, e.MediumtextField)
	assertField(t, "LongtextField", found.LongtextField, e.LongtextField)
	assertField(t, "EnumField", found.EnumField, e.EnumField)
	assertField(t, "SetField", found.SetField, e.SetField)
	assertField(t, "BinaryField", found.BinaryField, e.BinaryField)
	assertField(t, "VarbinaryField", found.VarbinaryField, e.VarbinaryField)
	assertField(t, "BlobField", found.BlobField, e.BlobField)
	assertField(t, "TinyblobField", found.TinyblobField, e.TinyblobField)
	assertField(t, "MediumblobField", found.MediumblobField, e.MediumblobField)
	assertField(t, "LongblobField", found.LongblobField, e.LongblobField)
	assertField(t, "DateField", found.DateField, e.DateField)
	assertField(t, "TimeField", found.TimeField, e.TimeField)
	assertField(t, "YearField", found.YearField, e.YearField)
	assertField(t, "DatetimeField", found.DatetimeField, e.DatetimeField)
	assertField(t, "TimestampField", found.TimestampField, e.TimestampField)
	assertField(t, "UuidField", found.UuidField, e.UuidField)
}

func TestQueryGetAllAnimals(t *testing.T) {
	resetTables(t)

	row := &Alpha.Entity{
		Uuid:        testUUID(),
		FirstInsert: "2025-06-30 12:00:00",
		LastUpdate:  "2025-06-30 12:00:00",
		Animal:      "cat",
		BigNumber:   "9000",
		TestField:   "test",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.Fields...))
	if result.Error != nil {
		t.Fatal("insert failed:", result.Error)
	}

	queryResult := Alpha.QueryGetAllAnimals()
	if queryResult.Error != nil {
		t.Fatal("query failed:", queryResult.Error)
	}
	for _, entity := range queryResult.Entities {
		if entity.Animal == "cat" && entity.BigNumber == "9000" {
			return
		}
	}
	t.Fatalf("expected row with Animal=cat and BigNumber=9000 not found: %+v", queryResult.Entities)
}

func TestQueryGetRecentCats(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	row := &Alpha.Entity{
		Uuid:        uuid,
		FirstInsert: "2025-06-30 12:00:00",
		LastUpdate:  "2025-06-30 13:00:00",
		Animal:      "cat",
		BigNumber:   "12345",
		TestField:   "recent",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.Fields...))
	if result.Error != nil {
		t.Fatal("insert failed:", result.Error)
	}

	queryResult := generated.QueryGetRecentCats()
	if queryResult.Error != nil {
		t.Fatal("query failed:", queryResult.Error)
	}
	for _, entity := range queryResult.Entities {
		if entity.Uuid == uuid {
			return
		}
	}
	t.Fatalf("expected row with uuid %s not found: %+v", uuid, queryResult.Entities)
}

func TestQueryGetByUuid(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	row := &Alpha.Entity{
		Uuid:        uuid,
		FirstInsert: "2025-06-30 15:00:00",
		LastUpdate:  "2025-06-30 15:00:00",
		Animal:      "dog",
		BigNumber:   "5555",
		TestField:   "unique",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(Alpha.Fields...))
	if result.Error != nil {
		t.Fatal("insert failed:", result.Error)
	}

	queryResult := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if queryResult.Error != nil {
		t.Fatal("query failed:", queryResult.Error)
	}
	if queryResult.Entity == nil || queryResult.Entity.Animal != "dog" || queryResult.Entity.TestField != "unique" {
		t.Fatalf("expected row with Animal=dog and TestField=unique, got: %+v", queryResult.Entity)
	}
}

func TestQueryCountBigNumbers(t *testing.T) {
	resetTables(t)

	row := &Alpha.Entity{
		Uuid:        testUUID(),
		FirstInsert: "2025-06-30 16:00:00",
		LastUpdate:  "2025-06-30 16:00:00",
		Animal:      "nulltest",
		TestField:   "checknull",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(
		Alpha.FieldUuid,
		Alpha.FieldFirstInsert,
		Alpha.FieldLastUpdate,
		Alpha.FieldAnimal,
		Alpha.FieldTestField,
	))
	if result.Error != nil {
		t.Fatal("insert failed:", result.Error)
	}

	queryResult := generated.QueryCountBigNumbers()
	if queryResult.Error != nil {
		t.Fatal("query failed:", queryResult.Error)
	}
	if queryResult.Entity == nil {
		t.Fatal("no result returned")
	}

	count, err := strconv.Atoi(queryResult.Entity.Count)
	if err != nil {
		t.Fatalf("invalid count returned: %v", queryResult.Entity.Count)
	}
	if count != 1 {
		t.Errorf("expected 1 row with NULL BigNumber, got: %d", count)
	}
}

func TestExecInsertOne(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	queryResult := generated.ExecInsertOne(generated.NewQueryParams().WithParams(uuid, "hedgehog", "tf"))
	if queryResult.Error != nil {
		t.Fatal("insert failed:", queryResult.Error)
	}

	result := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if result.Error != nil {
		t.Fatal("query failed:", result.Error)
	}
	if result.Entity == nil || result.Entity.Animal != "hedgehog" || result.Entity.TestField != "tf" {
		t.Fatalf("row not inserted as expected: %+v", result.Entity)
	}
}

func TestExecInsertHardcoded(t *testing.T) {
	resetTables(t)

	const uuid = "11111111-1111-4111-8111-111111111111"
	queryResult := generated.ExecInsertHardcoded()
	if queryResult.Error != nil {
		t.Fatal("insert hardcoded failed:", queryResult.Error)
	}

	result := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if result.Error != nil {
		t.Fatal("query failed:", result.Error)
	}
	if result.Entity == nil || result.Entity.Animal != "dog" {
		t.Fatalf("expected Animal=dog for hardcoded uuid, got: %+v", result.Entity)
	}
}

func TestExecUpdateAnimalName(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	row := &Alpha.Entity{
		Uuid:        uuid,
		FirstInsert: "2025-06-30 10:00:00",
		LastUpdate:  "2025-06-30 10:00:00",
		Animal:      "cat",
		TestField:   "x",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(
		Alpha.FieldUuid,
		Alpha.FieldFirstInsert,
		Alpha.FieldLastUpdate,
		Alpha.FieldAnimal,
		Alpha.FieldTestField,
	))
	if result.Error != nil {
		t.Fatal("seed insert failed:", result.Error)
	}

	queryResult := generated.ExecUpdateAnimalName(generated.NewQueryParams().WithParams("otter", uuid))
	if queryResult.Error != nil {
		t.Fatal("update failed:", queryResult.Error)
	}

	lookup := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if lookup.Error != nil {
		t.Fatal("query failed:", lookup.Error)
	}
	if lookup.Entity == nil || lookup.Entity.Animal != "otter" {
		t.Fatalf("expected Animal=otter after update, got: %+v", lookup.Entity)
	}
}

func TestExecUpdateTestField(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	row := &Alpha.Entity{
		Uuid:        uuid,
		FirstInsert: "2025-06-30 11:00:00",
		LastUpdate:  "2025-06-30 11:00:00",
		Animal:      "fox",
		TestField:   "old",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(
		Alpha.FieldUuid,
		Alpha.FieldFirstInsert,
		Alpha.FieldLastUpdate,
		Alpha.FieldAnimal,
		Alpha.FieldTestField,
	))
	if result.Error != nil {
		t.Fatal("seed insert failed:", result.Error)
	}

	queryResult := generated.ExecUpdateTestField()
	if queryResult.Error != nil {
		t.Fatal("update failed:", queryResult.Error)
	}

	lookup := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if lookup.Error != nil {
		t.Fatal("query failed:", lookup.Error)
	}
	if lookup.Entity == nil || lookup.Entity.TestField != "updated" {
		t.Fatalf("expected test_field=updated after bulk update, got: %+v", lookup.Entity)
	}
}

func TestExecDeleteByUuid(t *testing.T) {
	resetTables(t)

	uuid := testUUID()
	row := &Alpha.Entity{
		Uuid:        uuid,
		FirstInsert: "2025-06-30 12:00:00",
		LastUpdate:  "2025-06-30 12:00:00",
		Animal:      "toad",
		TestField:   "y",
	}
	result := row.DBInsert(Alpha.NewQueryParams().WithInsert(
		Alpha.FieldUuid,
		Alpha.FieldFirstInsert,
		Alpha.FieldLastUpdate,
		Alpha.FieldAnimal,
		Alpha.FieldTestField,
	))
	if result.Error != nil {
		t.Fatal("seed insert failed:", result.Error)
	}

	queryResult := generated.ExecDeleteByUuid(generated.NewQueryParams().WithParams(uuid))
	if queryResult.Error != nil {
		t.Fatal("delete failed:", queryResult.Error)
	}

	lookup := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(uuid))
	if lookup.Error != nil {
		t.Fatal("query failed:", lookup.Error)
	}
	if lookup.Entity != nil {
		t.Fatalf("expected no row after delete, got: %+v", lookup.Entity)
	}
}

func TestExecDeleteOldRows(t *testing.T) {
	resetTables(t)

	oldUUID := testUUID()
	newUUID := testUUID()
	oldRow := &Alpha.Entity{
		Uuid:        oldUUID,
		FirstInsert: "2022-12-31 23:59:59",
		LastUpdate:  "2022-12-31 23:59:59",
		Animal:      "ant",
		TestField:   "old",
	}
	newRow := &Alpha.Entity{
		Uuid:        newUUID,
		FirstInsert: "2025-01-01 00:00:01",
		LastUpdate:  "2025-01-01 00:00:01",
		Animal:      "bee",
		TestField:   "new",
	}
	fields := Alpha.NewQueryParams().WithInsert(
		Alpha.FieldUuid,
		Alpha.FieldFirstInsert,
		Alpha.FieldLastUpdate,
		Alpha.FieldAnimal,
		Alpha.FieldTestField,
	)
	if result := oldRow.DBInsert(fields); result.Error != nil {
		t.Fatal("seed old insert failed:", result.Error)
	}
	if result := newRow.DBInsert(fields); result.Error != nil {
		t.Fatal("seed new insert failed:", result.Error)
	}

	queryResult := generated.ExecDeleteOldRows()
	if queryResult.Error != nil {
		t.Fatal("delete old rows failed:", queryResult.Error)
	}

	oldLookup := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(oldUUID))
	if oldLookup.Error != nil {
		t.Fatal("query old failed:", oldLookup.Error)
	}
	if oldLookup.Entity != nil {
		t.Fatalf("expected old row to be deleted, got: %+v", oldLookup.Entity)
	}

	newLookup := generated.QueryGetByUuid(generated.NewQueryParams().WithParams(newUUID))
	if newLookup.Error != nil {
		t.Fatal("query new failed:", newLookup.Error)
	}
	if newLookup.Entity == nil || newLookup.Entity.Animal != "bee" {
		t.Fatalf("expected new row to remain, got: %+v", newLookup.Entity)
	}
}

func resetTables(t *testing.T) {
	t.Helper()

	checkTruncate(t, "all_types", AllTypes.DBTruncate().Error)
	checkTruncate(t, "alpha", Alpha.DBTruncate().Error)
	checkTruncate(t, "beta", Beta.DBTruncate().Error)
}

func checkTruncate(t *testing.T, table string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("truncate %s: %v", table, err)
	}
}

func testUUID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", identifierSeed.Add(1))
}

func assertField(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s mismatch: got %q, want %q", name, got, want)
	}
}
