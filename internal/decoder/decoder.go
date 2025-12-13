package decoder

//we need the channel order to be able to decode the data
//channel order because the raw buffer is a sequence of interleaved channels,
// and the only authoritative description of that interleaving is the scan index (Index)
// in the XML
