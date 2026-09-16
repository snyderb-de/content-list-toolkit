export namespace main {
	
	export class AgencyTemplateFields {
	    rg: string;
	    sg: string;
	    series: string;
	    rcSeries: string;
	    deptOrganization: string;
	    division: string;
	    section: string;
	    unit: string;
	    rcSeriesName: string;
	    beginDate: string;
	    endDate: string;
	    description: string;
	    location: string;
	    materialType: string;
	    comments: string;
	    confidential: string;
	    dispositionDate: string;
	    boxNum: string;
	    tdNum: string;
	    locationId: string;
	    recordLevel: string;
	
	    static createFrom(source: any = {}) {
	        return new AgencyTemplateFields(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rg = source["rg"];
	        this.sg = source["sg"];
	        this.series = source["series"];
	        this.rcSeries = source["rcSeries"];
	        this.deptOrganization = source["deptOrganization"];
	        this.division = source["division"];
	        this.section = source["section"];
	        this.unit = source["unit"];
	        this.rcSeriesName = source["rcSeriesName"];
	        this.beginDate = source["beginDate"];
	        this.endDate = source["endDate"];
	        this.description = source["description"];
	        this.location = source["location"];
	        this.materialType = source["materialType"];
	        this.comments = source["comments"];
	        this.confidential = source["confidential"];
	        this.dispositionDate = source["dispositionDate"];
	        this.boxNum = source["boxNum"];
	        this.tdNum = source["tdNum"];
	        this.locationId = source["locationId"];
	        this.recordLevel = source["recordLevel"];
	    }
	}
	export class CloneCompareOptions {
	    driveA: string;
	    driveB: string;
	    outputDir: string;
	    hashAlgorithm: string;
	    softCompare: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CloneCompareOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driveA = source["driveA"];
	        this.driveB = source["driveB"];
	        this.outputDir = source["outputDir"];
	        this.hashAlgorithm = source["hashAlgorithm"];
	        this.softCompare = source["softCompare"];
	    }
	}
	export class GettyCheckOptions {
	    sheetPath: string;
	    lastSheet?: string;
	    source: string;
	    vocabularyPath: string;
	    writeCleaned: boolean;
	    writeReport: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GettyCheckOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sheetPath = source["sheetPath"];
	        this.lastSheet = source["lastSheet"];
	        this.source = source["source"];
	        this.vocabularyPath = source["vocabularyPath"];
	        this.writeCleaned = source["writeCleaned"];
	        this.writeReport = source["writeReport"];
	    }
	}
	export class TagTermVerdict {
	    term: string;
	    checked: boolean;
	    found: boolean;
	    subjectId?: string;
	    preferredLabel?: string;
	    error?: string;
	    suggestions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new TagTermVerdict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.term = source["term"];
	        this.checked = source["checked"];
	        this.found = source["found"];
	        this.subjectId = source["subjectId"];
	        this.preferredLabel = source["preferredLabel"];
	        this.error = source["error"];
	        this.suggestions = source["suggestions"];
	    }
	}
	export class TagIssue {
	    kind: string;
	    severity: string;
	    repaired: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new TagIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.severity = source["severity"];
	        this.repaired = source["repaired"];
	        this.detail = source["detail"];
	    }
	}
	export class TagCheckResult {
	    original: string;
	    cleaned: string;
	    tags: string[];
	    issues: TagIssue[];
	    terms?: TagTermVerdict[];
	
	    static createFrom(source: any = {}) {
	        return new TagCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original = source["original"];
	        this.cleaned = source["cleaned"];
	        this.tags = source["tags"];
	        this.issues = this.convertValues(source["issues"], TagIssue);
	        this.terms = this.convertValues(source["terms"], TagTermVerdict);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TagRow {
	    number: number;
	    result: TagCheckResult;
	
	    static createFrom(source: any = {}) {
	        return new TagRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.result = this.convertValues(source["result"], TagCheckResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TagChange {
	    row: number;
	    before: string;
	    after: string;
	    byHand: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TagChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.row = source["row"];
	        this.before = source["before"];
	        this.after = source["after"];
	        this.byHand = source["byHand"];
	    }
	}
	export class TagSheetReport {
	    path: string;
	    format: string;
	    sheetName?: string;
	    columnLetter?: string;
	    columnIndex: number;
	    totalRows: number;
	    emptyCells: number;
	    vocabularySource?: string;
	    vocabularyNote?: string;
	    sourcePath?: string;
	    changes?: TagChange[];
	    rows: TagRow[];
	
	    static createFrom(source: any = {}) {
	        return new TagSheetReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.format = source["format"];
	        this.sheetName = source["sheetName"];
	        this.columnLetter = source["columnLetter"];
	        this.columnIndex = source["columnIndex"];
	        this.totalRows = source["totalRows"];
	        this.emptyCells = source["emptyCells"];
	        this.vocabularySource = source["vocabularySource"];
	        this.vocabularyNote = source["vocabularyNote"];
	        this.sourcePath = source["sourcePath"];
	        this.changes = this.convertValues(source["changes"], TagChange);
	        this.rows = this.convertValues(source["rows"], TagRow);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GettyCheckResult {
	    report: TagSheetReport;
	    summary: string;
	    cleanedPath?: string;
	    reportPath?: string;
	    elapsed: string;
	
	    static createFrom(source: any = {}) {
	        return new GettyCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.report = this.convertValues(source["report"], TagSheetReport);
	        this.summary = source["summary"];
	        this.cleanedPath = source["cleanedPath"];
	        this.reportPath = source["reportPath"];
	        this.elapsed = source["elapsed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GettyDownloadResult {
	    path: string;
	    terms: number;
	    archive: string;
	    published: string;
	    elapsed: string;
	
	    static createFrom(source: any = {}) {
	        return new GettyDownloadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.terms = source["terms"];
	        this.archive = source["archive"];
	        this.published = source["published"];
	        this.elapsed = source["elapsed"];
	    }
	}
	export class GettyReachability {
	    reachable: boolean;
	    // Go type: time
	    checkedAt: any;
	    latencyMs: number;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new GettyReachability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reachable = source["reachable"];
	        this.checkedAt = this.convertValues(source["checkedAt"], null);
	        this.latencyMs = source["latencyMs"];
	        this.detail = source["detail"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GettySaveResult {
	    cleanedPath: string;
	    report: TagSheetReport;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new GettySaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cleanedPath = source["cleanedPath"];
	        this.report = this.convertValues(source["report"], TagSheetReport);
	        this.summary = source["summary"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GettyTagEdit {
	    row: number;
	    tags: string;
	
	    static createFrom(source: any = {}) {
	        return new GettyTagEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.row = source["row"];
	        this.tags = source["tags"];
	    }
	}
	export class ScanOptions {
	    sourceDir: string;
	    outputDir: string;
	    outputFile: string;
	    hashAlgorithm: string;
	    excludeHidden: boolean;
	    excludeSystem: boolean;
	    createXLSX: boolean;
	    preserveZeros: boolean;
	    deleteCSV: boolean;
	    excludedExts: string;
	    foldersOnly: boolean;
	    folderDepth: number;
	    agencyTemplate: boolean;
	    agencyFields: AgencyTemplateFields;
	    releaseFolder: string;
	    gettySource?: string;
	    gettyVocabularyPath?: string;
	    gettyLastSheet?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceDir = source["sourceDir"];
	        this.outputDir = source["outputDir"];
	        this.outputFile = source["outputFile"];
	        this.hashAlgorithm = source["hashAlgorithm"];
	        this.excludeHidden = source["excludeHidden"];
	        this.excludeSystem = source["excludeSystem"];
	        this.createXLSX = source["createXLSX"];
	        this.preserveZeros = source["preserveZeros"];
	        this.deleteCSV = source["deleteCSV"];
	        this.excludedExts = source["excludedExts"];
	        this.foldersOnly = source["foldersOnly"];
	        this.folderDepth = source["folderDepth"];
	        this.agencyTemplate = source["agencyTemplate"];
	        this.agencyFields = this.convertValues(source["agencyFields"], AgencyTemplateFields);
	        this.releaseFolder = source["releaseFolder"];
	        this.gettySource = source["gettySource"];
	        this.gettyVocabularyPath = source["gettyVocabularyPath"];
	        this.gettyLastSheet = source["gettyLastSheet"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	export class UpdateStatus {
	    supported: boolean;
	    updateAvailable: boolean;
	    readyToRestart: boolean;
	    currentVersion: string;
	    latestVersion: string;
	    releaseFolder: string;
	    releasePath: string;
	    sha256: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.updateAvailable = source["updateAvailable"];
	        this.readyToRestart = source["readyToRestart"];
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.releaseFolder = source["releaseFolder"];
	        this.releasePath = source["releasePath"];
	        this.sha256 = source["sha256"];
	        this.message = source["message"];
	    }
	}

}

