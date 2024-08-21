package main

func main() {
	s := NewApiServer(":12358")
	panic(s.ListenAndServe())
}
