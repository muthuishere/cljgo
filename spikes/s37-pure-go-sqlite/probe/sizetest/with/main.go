// Hello WITH modernc.org/sqlite linked in (open a :memory: DB so the linker
// cannot dead-code-eliminate the driver). The size gap vs ../without is the
// battery's binary-size cost.
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	var n int
	if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
		panic(err)
	}
	fmt.Println("hello", n)
}
