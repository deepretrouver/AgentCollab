package sqlite

// Writes are intentionally serialized by Store.Write. Domain repositories must
// never call database write statements through Store.Read.
