package domain

// SchemaVersion is written on every persisted domain value (Record, Span,
// Finding) so that readers can reject or upgrade old shapes. Bump when any
// field changes meaning or disappears; new optional fields do not require
// a bump as long as zero values remain valid.
const SchemaVersion uint16 = 1
