package main

type ScanOptions struct {
	SourceDir      string               `json:"sourceDir"`
	OutputDir      string               `json:"outputDir"`
	OutputFile     string               `json:"outputFile"`
	HashAlgorithm  string               `json:"hashAlgorithm"`
	ExcludeHidden  bool                 `json:"excludeHidden"`
	ExcludeSystem  bool                 `json:"excludeSystem"`
	CreateXLSX     bool                 `json:"createXLSX"`
	PreserveZeros  bool                 `json:"preserveZeros"`
	DeleteCSV      bool                 `json:"deleteCSV"`
	ExcludedExts   string               `json:"excludedExts"`
	FoldersOnly    bool                 `json:"foldersOnly"`
	FolderDepth    int                  `json:"folderDepth"`
	AgencyTemplate bool                 `json:"agencyTemplate"`
	AgencyFields   AgencyTemplateFields `json:"agencyFields"`
	ReleaseFolder  string               `json:"releaseFolder"`
	// GettySource and GettyVocabularyPath remember the Getty Tag screen's
	// choice of where to verify terms.
	GettySource         string `json:"gettySource,omitempty"`
	GettyVocabularyPath string `json:"gettyVocabularyPath,omitempty"`
	// GettyLastSheet is the export checked last, so the picker opens where the
	// work is rather than at the home directory every time.
	GettyLastSheet string `json:"gettyLastSheet,omitempty"`
}

type AgencyTemplateFields struct {
	RG               string `json:"rg"`
	SG               string `json:"sg"`
	Series           string `json:"series"`
	RCSeries         string `json:"rcSeries"`
	DeptOrganization string `json:"deptOrganization"`
	Division         string `json:"division"`
	Section          string `json:"section"`
	Unit             string `json:"unit"`
	RCSeriesName     string `json:"rcSeriesName"`
	BeginDate        string `json:"beginDate"`
	EndDate          string `json:"endDate"`
	Description      string `json:"description"`
	Location         string `json:"location"`
	MaterialType     string `json:"materialType"`
	Comments         string `json:"comments"`
	Confidential     string `json:"confidential"`
	DispositionDate  string `json:"dispositionDate"`
	BoxNum           string `json:"boxNum"`
	TDNum            string `json:"tdNum"`
	LocationID       string `json:"locationId"`
	RecordLevel      string `json:"recordLevel"`
}

type AppSettings struct {
	HashAlgorithm  string               `json:"hashAlgorithm"`
	ExcludeHidden  bool                 `json:"excludeHidden"`
	ExcludeSystem  bool                 `json:"excludeSystem"`
	CreateXLSX     bool                 `json:"createXLSX"`
	PreserveZeros  bool                 `json:"preserveZeros"`
	DeleteCSV      bool                 `json:"deleteCSV"`
	ExcludedExts   string               `json:"excludedExts"`
	FoldersOnly    bool                 `json:"foldersOnly"`
	FolderDepth    int                  `json:"folderDepth"`
	AgencyTemplate bool                 `json:"agencyTemplate"`
	AgencyFields   AgencyTemplateFields `json:"agencyFields"`
	ReleaseFolder  string               `json:"releaseFolder"`
	// GettySource and GettyVocabularyPath remember the Getty Tag screen's
	// choice of where to verify terms.
	GettySource         string `json:"gettySource,omitempty"`
	GettyVocabularyPath string `json:"gettyVocabularyPath,omitempty"`
	// GettyLastSheet is the export checked last, so the picker opens where the
	// work is rather than at the home directory every time.
	GettyLastSheet string `json:"gettyLastSheet,omitempty"`
}

// defaultAppSettings is the baseline used whenever settings are missing or
// unreadable, so a corrupt file degrades to sane defaults rather than zero
// values.
func defaultAppSettings() AppSettings {
	return AppSettings{
		HashAlgorithm: string(defaultHashAlgorithm()),
		ExcludeHidden: true,
		ExcludeSystem: true,
		CreateXLSX:    true,
		PreserveZeros: true,
		DeleteCSV:     true,
		AgencyFields:  defaultAgencyTemplateFields(),
		ReleaseFolder: defaultReleaseFolder,
	}
}

type UpdateStatus struct {
	Supported       bool   `json:"supported"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReadyToRestart  bool   `json:"readyToRestart"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	ReleaseFolder   string `json:"releaseFolder"`
	ReleasePath     string `json:"releasePath"`
	SHA256          string `json:"sha256"`
	Message         string `json:"message"`
}

type SummaryEntry struct {
	Label string `json:"label"`
	Count uint64 `json:"count"`
	Bytes uint64 `json:"bytes"`
}

type ScanProgressPayload struct {
	Phase       string  `json:"phase"`
	Files       uint64  `json:"files"`
	Directories uint64  `json:"directories"`
	Bytes       uint64  `json:"bytes"`
	Filtered    uint64  `json:"filtered"`
	TotalFiles  uint64  `json:"totalFiles"`
	TotalDirs   uint64  `json:"totalDirs"`
	TotalBytes  uint64  `json:"totalBytes"`
	CurrentItem string  `json:"currentItem"`
	Percent     float64 `json:"percent"`
	ETASecs     float64 `json:"etaSecs"`
	ElapsedSecs float64 `json:"elapsedSecs"`
}

type ScanDonePayload struct {
	Files           uint64         `json:"files"`
	Directories     uint64         `json:"directories"`
	Bytes           uint64         `json:"bytes"`
	Errors          uint64         `json:"errors"`
	Filtered        uint64         `json:"filtered"`
	FilteredHidden  uint64         `json:"filteredHidden"`
	FilteredSystem  uint64         `json:"filteredSystem"`
	FilteredExts    uint64         `json:"filteredExts"`
	FilteredOSNoise uint64         `json:"filteredOSNoise"`
	SourceName      string         `json:"sourceName"`
	OutputPath      string         `json:"outputPath"`
	OutputPaths     []string       `json:"outputPaths"`
	XLSXPath        string         `json:"xlsxPath"`
	XLSXPaths       []string       `json:"xlsxPaths"`
	ReportPath      string         `json:"reportPath"`
	ElapsedSecs     float64        `json:"elapsedSecs"`
	TopByCount      []SummaryEntry `json:"topByCount"`
	TopBySize       []SummaryEntry `json:"topBySize"`
	HashWorkers     int            `json:"hashWorkers"`
	HashAlgorithm   string         `json:"hashAlgorithm"`
	CreateXLSX      bool           `json:"createXLSX"`
	PreserveZeros   bool           `json:"preserveZeros"`
	DeleteCSV       bool           `json:"deleteCSV"`
	CSVDeleted      bool           `json:"csvDeleted"`
	MaxRowsPerCSV   uint64         `json:"maxRowsPerCSV"`
	CSVPartCount    int            `json:"csvPartCount"`
	XLSXPartCount   int            `json:"xlsxPartCount"`
	FilteredSamples []string       `json:"filteredSamples"`
	FirstCSVItem    string         `json:"firstCSVItem"`
	LastCSVItem     string         `json:"lastCSVItem"`
	FoldersOnly     bool           `json:"foldersOnly"`
}

type EmailProgressPayload struct {
	Phase   string `json:"phase"`
	Copied  uint64 `json:"copied"`
	Scanned uint64 `json:"scanned"`
	Matched uint64 `json:"matched"`
	Total   uint64 `json:"total"`
}

type EmailDonePayload struct {
	SourceDir    string  `json:"sourceDir"`
	DestDir      string  `json:"destDir"`
	ManifestPath string  `json:"manifestPath"`
	Copied       uint64  `json:"copied"`
	ElapsedSecs  float64 `json:"elapsedSecs"`
}

type CloneCompareOptions struct {
	DriveA        string `json:"driveA"`
	DriveB        string `json:"driveB"`
	OutputDir     string `json:"outputDir"`
	HashAlgorithm string `json:"hashAlgorithm"`
	SoftCompare   bool   `json:"softCompare"`
}

type CloneProgressPayload struct {
	Phase       string  `json:"phase"`
	SubPhase    string  `json:"subPhase"`
	Percent     float64 `json:"percent"`
	Compared    uint64  `json:"compared"`
	Total       uint64  `json:"total"`
	Differences uint64  `json:"differences"`
	CurrentItem string  `json:"currentItem"`
	Files       uint64  `json:"files"`
	TotalFiles  uint64  `json:"totalFiles"`
	Bytes       uint64  `json:"bytes"`
	TotalBytes  uint64  `json:"totalBytes"`
	BytesPerSec float64 `json:"bytesPerSec"`
	ETASecs     float64 `json:"etaSecs"`
	ElapsedSecs float64 `json:"elapsedSecs"`
}

type DiffRowPayload struct {
	DiffType string `json:"diffType"`
	PathA    string `json:"pathA"`
	PathB    string `json:"pathB"`
	SizeA    string `json:"sizeA"`
	SizeB    string `json:"sizeB"`
	HashA    string `json:"hashA"`
	HashB    string `json:"hashB"`
}

type CloneDonePayload struct {
	DiffPath          string  `json:"diffPath"`
	ReportPath        string  `json:"reportPath"`
	HashAlgorithm     string  `json:"hashAlgorithm"`
	ElapsedSecs       float64 `json:"elapsedSecs"`
	Verdict           string  `json:"verdict"`
	Compared          uint64  `json:"compared"`
	Differences       uint64  `json:"differences"`
	MovedFiles        uint64  `json:"movedFiles"`
	DuplicatesOnB     uint64  `json:"duplicatesOnB"`
	DuplicatesOnA     uint64  `json:"duplicatesOnA"`
	MissingNoMatch    uint64  `json:"missingNoMatch"`
	ExtraNoMatch      uint64  `json:"extraNoMatch"`
	SizeMismatches    uint64  `json:"sizeMismatches"`
	HashMismatches    uint64  `json:"hashMismatches"`
	ExcludedSystem    uint64  `json:"excludedSystem"`
	MetadataOnlyDiffs uint64  `json:"metadataOnlyDiffs"`
	SoftCompare       bool    `json:"softCompare"`
}
