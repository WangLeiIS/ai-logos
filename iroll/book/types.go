package book

type Manifest struct {
	Format        string   `json:"format"`
	FormatVersion int      `json:"format_version"`
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Authors       []string `json:"authors"`
	Language      string   `json:"language"`
	Tags          []string `json:"tags"`
	ChunkCount    int64    `json:"chunk_count"`
	SearchEngine  string   `json:"search_engine"`
}

type ChunkRow struct {
	ChunkID         string   `parquet:"chunk_id,optional"`
	BookID          string   `parquet:"book_id,optional"`
	ChunkType       string   `parquet:"chunk_type,optional"`
	Content         string   `parquet:"content,optional"`
	Questions       []string `parquet:"questions,list,optional"`
	TitleKeywords   []string `parquet:"title_keywords,list,optional"`
	ContentKeywords []string `parquet:"content_keywords,list,optional"`
	QuoteKeywords   []string `parquet:"quote_keywords,list,optional"`
	SeqNum          int32    `parquet:"seq_num,optional"`
	SourceFile      string   `parquet:"source_file,optional"`
	StartLine       int32    `parquet:"start_line,optional"`
	EndLine         int32    `parquet:"end_line,optional"`
	CharCount       int32    `parquet:"char_count,optional"`
	Summary         string   `parquet:"summary,optional"`
}

type IndexRow struct {
	ID        string `parquet:"id,optional"`
	ChunkID   string `parquet:"chunk_id,optional"`
	Keyword   string `parquet:"keyword,optional"`
	FieldType string `parquet:"field_type,optional"`
}

type IDFRow struct {
	Keyword string  `parquet:"keyword,optional"`
	DF      int32   `parquet:"df,optional"`
	IDF     float64 `parquet:"idf,optional"`
}

type Bundle struct {
	Dir          string   `json:"dir"`
	ResourcePath string   `json:"resource_path"`
	Manifest     Manifest `json:"manifest"`
}

type Book struct {
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	ResourcePath  string   `json:"resource_path"`
	FormatVersion int      `json:"format_version"`
	Authors       []string `json:"authors"`
	Language      string   `json:"language"`
	Tags          []string `json:"tags"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type Query struct {
	Books        []string `json:"books"`
	Tags         []string `json:"tags"`
	Limit        int      `json:"limit"`
	PerBookLimit int      `json:"per_book_limit"`
}

type Result struct {
	BookID          string   `json:"book_id"`
	ChunkID         string   `json:"chunk_id"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	SourcePath      string   `json:"source_path"`
	Position        int64    `json:"position"`
	Score           float64  `json:"score"`
	TitleCoverage   float64  `json:"title_coverage"`
	ContentCoverage float64  `json:"content_coverage"`
	QuoteCoverage   float64  `json:"quote_coverage"`
	AvgIDF          float64  `json:"avg_idf"`
	MatchedKeywords []string `json:"matched_keywords"`
}

type QueryResponse struct {
	QueryTags []string `json:"query_tags"`
	Books     []string `json:"books"`
	Results   []Result `json:"results"`
}
