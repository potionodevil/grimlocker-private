//go:build ignore

package main
import ("fmt";"os";"github.com/grimlocker/grimdb/storage/grimdb")
func main() {
    _, err := grimdb.InitializeVault("TestPass!", os.Args[1])
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
    fmt.Println("OK")
}
