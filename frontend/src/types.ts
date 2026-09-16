export interface ScanOptions {
  sourceDir: string
  outputDir: string
  outputFile: string
  hashAlgorithm: string
  excludeHidden: boolean
  excludeSystem: boolean
  createXLSX: boolean
  preserveZeros: boolean
  deleteCSV: boolean
  excludedExts: string
  foldersOnly: boolean
  folderDepth: number
  agencyTemplate: boolean
  agencyFields: AgencyTemplateFields
  releaseFolder: string
}

export interface AgencyTemplateFields {
  rg: string
  sg: string
  series: string
  rcSeries: string
  deptOrganization: string
  division: string
  section: string
  unit: string
  rcSeriesName: string
  beginDate: string
  endDate: string
  description: string
  location: string
  materialType: string
  comments: string
  confidential: string
  dispositionDate: string
  boxNum: string
  tdNum: string
  locationId: string
  recordLevel: string
}

export interface AppSettings {
  hashAlgorithm: string
  excludeHidden: boolean
  excludeSystem: boolean
  createXLSX: boolean
  preserveZeros: boolean
  deleteCSV: boolean
  excludedExts: string
  foldersOnly: boolean
  folderDepth: number
  agencyTemplate: boolean
  agencyFields: AgencyTemplateFields
  releaseFolder: string
}

export interface UpdateStatus {
  supported: boolean
  updateAvailable: boolean
  readyToRestart: boolean
  currentVersion: string
  latestVersion: string
  releaseFolder: string
  releasePath: string
  sha256: string
  message: string
}

export interface SummaryEntry {
  label: string
  count: number
  bytes: number
}

export interface ScanProgressPayload {
  phase: string
  files: number
  directories: number
  bytes: number
  filtered: number
  totalFiles: number
  totalDirs: number
  totalBytes: number
  currentItem: string
  percent: number
  etaSecs: number
  elapsedSecs: number
}

export interface ScanDonePayload {
  files: number
  directories: number
  bytes: number
  errors: number
  filtered: number
  filteredHidden: number
  filteredSystem: number
  filteredExts: number
  filteredOSNoise: number
  sourceName: string
  outputPath: string
  outputPaths: string[]
  xlsxPath: string
  xlsxPaths: string[]
  reportPath: string
  elapsedSecs: number
  topByCount: SummaryEntry[]
  topBySize: SummaryEntry[]
  hashWorkers: number
  hashAlgorithm: string
  createXLSX: boolean
  preserveZeros: boolean
  deleteCSV: boolean
  csvDeleted: boolean
  maxRowsPerCSV: number
  csvPartCount: number
  xlsxPartCount: number
  filteredSamples: string[]
  firstCSVItem: string
  lastCSVItem: string
  foldersOnly: boolean
}

export interface EmailProgressPayload {
  phase: string
  copied: number
  scanned: number
  matched: number
  total: number
}

export interface EmailDonePayload {
  sourceDir: string
  destDir: string
  manifestPath: string
  copied: number
  elapsedSecs: number
}

export interface CloneCompareOptions {
  driveA: string
  driveB: string
  outputDir: string
  hashAlgorithm: string
  softCompare: boolean
}

export interface CloneProgressPayload {
  phase: string
  subPhase: string
  percent: number
  compared: number
  total: number
  differences: number
  currentItem: string
  files: number
  totalFiles: number
  bytes: number
  totalBytes: number
  bytesPerSec: number
  etaSecs: number
  elapsedSecs: number
}

export interface DiffRowPayload {
  diffType: string
  pathA: string
  pathB: string
  sizeA: string
  sizeB: string
  hashA: string
  hashB: string
}

export interface CloneDonePayload {
  diffPath: string
  reportPath: string
  hashAlgorithm: string
  elapsedSecs: number
  verdict: string
  compared: number
  differences: number
  movedFiles: number
  duplicatesOnB: number
  duplicatesOnA: number
  missingNoMatch: number
  extraNoMatch: number
  sizeMismatches: number
  hashMismatches: number
  excludedSystem: number
  metadataOnlyDiffs: number
  softCompare: boolean
}

export const HASH_ALGORITHMS = [
  { value: 'blake3', label: 'Fast (BLAKE3)' },
  { value: 'sha1',   label: 'Medium (SHA-1)' },
  { value: 'sha256', label: 'Strong (SHA-256)' },
  { value: 'off',    label: 'No hash' },
]

export type Screen = 'content-list' | 'email-copy' | 'clone-compare' | 'getty-tag' | 'manual' | 'about'
