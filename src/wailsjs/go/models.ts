export namespace docker {
	
	export class Capabilities {
	    docker: boolean;
	    dockerDaemon: boolean;
	    hadolint: boolean;
	    trivy: boolean;
	    grype: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.docker = source["docker"];
	        this.dockerDaemon = source["dockerDaemon"];
	        this.hadolint = source["hadolint"];
	        this.trivy = source["trivy"];
	        this.grype = source["grype"];
	    }
	}
	export class Finding {
	    rule: string;
	    severity: string;
	    message: string;
	    line: number;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.severity = source["severity"];
	        this.message = source["message"];
	        this.line = source["line"];
	    }
	}
	export class Stage {
	    name: string;
	    baseImage: string;
	    tag: string;
	    digest?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Stage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.baseImage = source["baseImage"];
	        this.tag = source["tag"];
	        this.digest = source["digest"];
	        this.final = source["final"];
	    }
	}
	export class Dockerfile {
	    path: string;
	    stages: Stage[];
	    exposedPorts: string[];
	    user: string;
	    hasHealthcheck: boolean;
	    findings: Finding[];
	
	    static createFrom(source: any = {}) {
	        return new Dockerfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.stages = this.convertValues(source["stages"], Stage);
	        this.exposedPorts = source["exposedPorts"];
	        this.user = source["user"];
	        this.hasHealthcheck = source["hasHealthcheck"];
	        this.findings = this.convertValues(source["findings"], Finding);
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
	
	export class Image {
	    repository: string;
	    tag: string;
	    id: string;
	    size: string;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Image(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repository = source["repository"];
	        this.tag = source["tag"];
	        this.id = source["id"];
	        this.size = source["size"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class Layer {
	    createdBy: string;
	    size: string;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Layer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.createdBy = source["createdBy"];
	        this.size = source["size"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class Project {
	    dockerfilePath: string;
	    composePath?: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dockerfilePath = source["dockerfilePath"];
	        this.composePath = source["composePath"];
	    }
	}
	
	export class Vulnerability {
	    id: string;
	    package: string;
	    severity: string;
	    installed: string;
	    fixedVersion: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new Vulnerability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.package = source["package"];
	        this.severity = source["severity"];
	        this.installed = source["installed"];
	        this.fixedVersion = source["fixedVersion"];
	        this.title = source["title"];
	    }
	}

}

export namespace gh {
	
	export class Label {
	    name: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new Label(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class Issue {
	    number: number;
	    title: string;
	    author: string;
	    url: string;
	    state: string;
	    labels: Label[];
	    assignees: string[];
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.title = source["title"];
	        this.author = source["author"];
	        this.url = source["url"];
	        this.state = source["state"];
	        this.labels = this.convertValues(source["labels"], Label);
	        this.assignees = source["assignees"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	
	export class PullRequest {
	    number: number;
	    title: string;
	    author: string;
	    url: string;
	    state: string;
	    draft: boolean;
	    reviewDecision: string;
	    labels: Label[];
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new PullRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.title = source["title"];
	        this.author = source["author"];
	        this.url = source["url"];
	        this.state = source["state"];
	        this.draft = source["draft"];
	        this.reviewDecision = source["reviewDecision"];
	        this.labels = this.convertValues(source["labels"], Label);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class WorkflowDispatchInput {
	    name: string;
	    description: string;
	    type: string;
	    required: boolean;
	    default: string;
	    options: string[];
	
	    static createFrom(source: any = {}) {
	        return new WorkflowDispatchInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.default = source["default"];
	        this.options = source["options"];
	    }
	}
	export class WorkflowDispatchSpec {
	    dispatchable: boolean;
	    inputs: WorkflowDispatchInput[];
	
	    static createFrom(source: any = {}) {
	        return new WorkflowDispatchSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dispatchable = source["dispatchable"];
	        this.inputs = this.convertValues(source["inputs"], WorkflowDispatchInput);
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
	export class WorkflowRun {
	    id: number;
	    workflowId: number;
	    name: string;
	    status: string;
	    conclusion: string;
	    url: string;
	    branch: string;
	    event: string;
	    createdAt: number;
	    runStartedAt?: number;
	    updatedAt: number;
	    prNumbers?: number[];
	
	    static createFrom(source: any = {}) {
	        return new WorkflowRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflowId = source["workflowId"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.conclusion = source["conclusion"];
	        this.url = source["url"];
	        this.branch = source["branch"];
	        this.event = source["event"];
	        this.createdAt = source["createdAt"];
	        this.runStartedAt = source["runStartedAt"];
	        this.updatedAt = source["updatedAt"];
	        this.prNumbers = source["prNumbers"];
	    }
	}
	export class WorkflowRunsPage {
	    runs: WorkflowRun[];
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowRunsPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runs = this.convertValues(source["runs"], WorkflowRun);
	        this.hasMore = source["hasMore"];
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

}

export namespace git {
	
	export class AgentState {
	    branch: string;
	    stagedCount: number;
	    aheadCount: number;
	    behindCount: number;
	    hasUpstream: boolean;
	    isProtected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.branch = source["branch"];
	        this.stagedCount = source["stagedCount"];
	        this.aheadCount = source["aheadCount"];
	        this.behindCount = source["behindCount"];
	        this.hasUpstream = source["hasUpstream"];
	        this.isProtected = source["isProtected"];
	    }
	}
	export class BranchInfo {
	    name: string;
	    isCurrent: boolean;
	    worktreePath?: string;
	    upstream?: string;
	
	    static createFrom(source: any = {}) {
	        return new BranchInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isCurrent = source["isCurrent"];
	        this.worktreePath = source["worktreePath"];
	        this.upstream = source["upstream"];
	    }
	}
	export class FileChangeStatus {
	    path: string;
	    status: string;
	    staged: boolean;
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new FileChangeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.staged = source["staged"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	export class ProviderToken {
	    provider: string;
	    token: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderToken(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.token = source["token"];
	        this.source = source["source"];
	    }
	}
	export class Remote {
	    provider: string;
	    host: string;
	    baseUrl: string;
	    owner: string;
	    repo: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Remote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.host = source["host"];
	        this.baseUrl = source["baseUrl"];
	        this.owner = source["owner"];
	        this.repo = source["repo"];
	        this.url = source["url"];
	    }
	}

}

export namespace jira {
	
	export class BoardInfo {
	    id: number;
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new BoardInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class Column {
	    name: string;
	    statusIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.statusIds = source["statusIds"];
	    }
	}
	export class Comment {
	    id: string;
	    author: string;
	    body: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.author = source["author"];
	        this.body = source["body"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Config {
	    baseUrl: string;
	    email: string;
	    token: string;
	    projectKey: string;
	    boardId?: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.email = source["email"];
	        this.token = source["token"];
	        this.projectKey = source["projectKey"];
	        this.boardId = source["boardId"];
	    }
	}
	export class CreateIssueInput {
	    summary: string;
	    issueTypeId: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateIssueInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.issueTypeId = source["issueTypeId"];
	    }
	}
	export class HistoryChange {
	    field: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class HistoryEntry {
	    id: string;
	    author: string;
	    createdAt: number;
	    changes: HistoryChange[];
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.author = source["author"];
	        this.createdAt = source["createdAt"];
	        this.changes = this.convertValues(source["changes"], HistoryChange);
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
	export class Issue {
	    key: string;
	    summary: string;
	    issueType: string;
	    priority: string;
	    status: string;
	    statusId: string;
	    statusCategory: string;
	    assignee: string;
	    assigneeEmail: string;
	    labels: string[];
	    url: string;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.summary = source["summary"];
	        this.issueType = source["issueType"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	        this.statusId = source["statusId"];
	        this.statusCategory = source["statusCategory"];
	        this.assignee = source["assignee"];
	        this.assigneeEmail = source["assigneeEmail"];
	        this.labels = source["labels"];
	        this.url = source["url"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class IssueDetail {
	    key: string;
	    summary: string;
	    description: string;
	    issueType: string;
	    priority: string;
	    status: string;
	    statusCategory: string;
	    assignee: string;
	    assigneeEmail: string;
	    reporter: string;
	    reporterEmail: string;
	    labels: string[];
	    url: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new IssueDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.summary = source["summary"];
	        this.description = source["description"];
	        this.issueType = source["issueType"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	        this.statusCategory = source["statusCategory"];
	        this.assignee = source["assignee"];
	        this.assigneeEmail = source["assigneeEmail"];
	        this.reporter = source["reporter"];
	        this.reporterEmail = source["reporterEmail"];
	        this.labels = source["labels"];
	        this.url = source["url"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class IssueType {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new IssueType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Sprint {
	    id: number;
	    name: string;
	    boardId: number;
	    columns: Column[];
	    issues: Issue[];
	
	    static createFrom(source: any = {}) {
	        return new Sprint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.boardId = source["boardId"];
	        this.columns = this.convertValues(source["columns"], Column);
	        this.issues = this.convertValues(source["issues"], Issue);
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

}

export namespace main {
	
	export class AgentCli {
	    kind: string;
	    binary: string;
	    installed: boolean;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentCli(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.binary = source["binary"];
	        this.installed = source["installed"];
	        this.path = source["path"];
	    }
	}
	export class BackendStatus {
	    ready: boolean;
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new BackendStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.lastError = source["lastError"];
	    }
	}
	export class CodeEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new CodeEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	    }
	}
	export class Ide {
	    id: string;
	    installed: boolean;
	    binary?: string;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new Ide(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.installed = source["installed"];
	        this.binary = source["binary"];
	        this.path = source["path"];
	    }
	}

}

export namespace nodejs {
	
	export class DepLocation {
	    workspace: string;
	    manifest: string;
	    version: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new DepLocation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspace = source["workspace"];
	        this.manifest = source["manifest"];
	        this.version = source["version"];
	        this.type = source["type"];
	    }
	}
	export class Dependency {
	    name: string;
	    version: string;
	    type: string;
	    locations?: DepLocation[];
	
	    static createFrom(source: any = {}) {
	        return new Dependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.locations = this.convertValues(source["locations"], DepLocation);
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
	export class OutdatedPackage {
	    name: string;
	    current: string;
	    wanted: string;
	    latest: string;
	    workspace?: string;
	
	    static createFrom(source: any = {}) {
	        return new OutdatedPackage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.current = source["current"];
	        this.wanted = source["wanted"];
	        this.latest = source["latest"];
	        this.workspace = source["workspace"];
	    }
	}
	export class Script {
	    name: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new Script(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	    }
	}
	export class Project {
	    manifestPath: string;
	    packageManager: string;
	    scripts: Script[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manifestPath = source["manifestPath"];
	        this.packageManager = source["packageManager"];
	        this.scripts = this.convertValues(source["scripts"], Script);
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
	
	export class UnusedPackage {
	    name: string;
	    workspace: string;
	
	    static createFrom(source: any = {}) {
	        return new UnusedPackage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.workspace = source["workspace"];
	    }
	}
	export class Vulnerability {
	    name: string;
	    severity: string;
	    title: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Vulnerability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.url = source["url"];
	    }
	}
	export class Workspace {
	    name: string;
	    path: string;
	    manifest: string;
	    isRoot: boolean;
	    dependencies: Dependency[];
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.manifest = source["manifest"];
	        this.isRoot = source["isRoot"];
	        this.dependencies = this.convertValues(source["dependencies"], Dependency);
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

}

export namespace polaris {
	
	export class ActionResult {
	    kind: string;
	    status: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	    }
	}
	export class Agent {
	    id: string;
	    projectId: string;
	    kind: string;
	    summary: string;
	    status: string;
	    startedAt: number;
	    tokens: number;
	    tokensInput: number;
	    tokensOutput: number;
	    tokensCacheCreate: number;
	    tokensCacheRead: number;
	    sessionId: string;
	    source: string;
	    costUsd: number;
	    filesModified: number;
	    toolsUsed: number;
	    branch?: string;
	    worktreePath?: string;
	    issueKey?: string;
	    prUrl?: string;
	    model?: string;
	    pendingQuestionId?: string;
	    pendingQuestionInput?: string;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.kind = source["kind"];
	        this.summary = source["summary"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	        this.tokens = source["tokens"];
	        this.tokensInput = source["tokensInput"];
	        this.tokensOutput = source["tokensOutput"];
	        this.tokensCacheCreate = source["tokensCacheCreate"];
	        this.tokensCacheRead = source["tokensCacheRead"];
	        this.sessionId = source["sessionId"];
	        this.source = source["source"];
	        this.costUsd = source["costUsd"];
	        this.filesModified = source["filesModified"];
	        this.toolsUsed = source["toolsUsed"];
	        this.branch = source["branch"];
	        this.worktreePath = source["worktreePath"];
	        this.issueKey = source["issueKey"];
	        this.prUrl = source["prUrl"];
	        this.model = source["model"];
	        this.pendingQuestionId = source["pendingQuestionId"];
	        this.pendingQuestionInput = source["pendingQuestionInput"];
	    }
	}
	export class CustomTheme {
	    key: string;
	    name: string;
	    mode: string;
	    colors: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new CustomTheme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.mode = source["mode"];
	        this.colors = source["colors"];
	    }
	}
	export class AppearanceSettings {
	    theme: string;
	    thinkingStyle: string;
	    customThemes: CustomTheme[];
	
	    static createFrom(source: any = {}) {
	        return new AppearanceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.thinkingStyle = source["thinkingStyle"];
	        this.customThemes = this.convertValues(source["customThemes"], CustomTheme);
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
	export class AutomationAction {
	    kind: string;
	    agentKind?: string;
	    model?: string;
	    taskTemplate?: string;
	    jiraIssueKey?: string;
	    jiraToStatusId?: string;
	    notifyTitle?: string;
	    notifyKind?: string;
	
	    static createFrom(source: any = {}) {
	        return new AutomationAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.agentKind = source["agentKind"];
	        this.model = source["model"];
	        this.taskTemplate = source["taskTemplate"];
	        this.jiraIssueKey = source["jiraIssueKey"];
	        this.jiraToStatusId = source["jiraToStatusId"];
	        this.notifyTitle = source["notifyTitle"];
	        this.notifyKind = source["notifyKind"];
	    }
	}
	export class AutomationTrigger {
	    kind: string;
	    fromStatusIds?: string[];
	    toStatusId?: string;
	    assignee?: string;
	    alsoOnReassignment?: boolean;
	    includeDrafts?: boolean;
	    authorFilter?: string;
	    excludeOwnComments?: boolean;
	    onlyMine?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AutomationTrigger(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.fromStatusIds = source["fromStatusIds"];
	        this.toStatusId = source["toStatusId"];
	        this.assignee = source["assignee"];
	        this.alsoOnReassignment = source["alsoOnReassignment"];
	        this.includeDrafts = source["includeDrafts"];
	        this.authorFilter = source["authorFilter"];
	        this.excludeOwnComments = source["excludeOwnComments"];
	        this.onlyMine = source["onlyMine"];
	    }
	}
	export class Automation {
	    id: string;
	    projectId: string;
	    name: string;
	    enabled: boolean;
	    source: string;
	    trigger: AutomationTrigger;
	    actions: AutomationAction[];
	    pollIntervalSec: number;
	    snapshotJson?: string;
	
	    static createFrom(source: any = {}) {
	        return new Automation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.source = source["source"];
	        this.trigger = this.convertValues(source["trigger"], AutomationTrigger);
	        this.actions = this.convertValues(source["actions"], AutomationAction);
	        this.pollIntervalSec = source["pollIntervalSec"];
	        this.snapshotJson = source["snapshotJson"];
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
	
	export class AutomationRun {
	    id: string;
	    automationId: string;
	    projectId: string;
	    startedAt: number;
	    outcome: string;
	    reason: string;
	    actions: ActionResult[];
	
	    static createFrom(source: any = {}) {
	        return new AutomationRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.automationId = source["automationId"];
	        this.projectId = source["projectId"];
	        this.startedAt = source["startedAt"];
	        this.outcome = source["outcome"];
	        this.reason = source["reason"];
	        this.actions = this.convertValues(source["actions"], ActionResult);
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
	
	export class ClaudeUsage {
	    sessionPercentUsed: number;
	    sessionResetAt?: string;
	    weeklyPercentUsed: number;
	    weeklyResetAt?: string;
	    lastUpdated: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClaudeUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionPercentUsed = source["sessionPercentUsed"];
	        this.sessionResetAt = source["sessionResetAt"];
	        this.weeklyPercentUsed = source["weeklyPercentUsed"];
	        this.weeklyResetAt = source["weeklyResetAt"];
	        this.lastUpdated = source["lastUpdated"];
	        this.error = source["error"];
	    }
	}
	export class CustomProvider {
	    id: string;
	    name: string;
	    color: string;
	    endpoint: string;
	    apiKey: string;
	    apiType: string;
	    models: string[];
	
	    static createFrom(source: any = {}) {
	        return new CustomProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.endpoint = source["endpoint"];
	        this.apiKey = source["apiKey"];
	        this.apiType = source["apiType"];
	        this.models = source["models"];
	    }
	}
	
	export class GeneralSettings {
	    autoResumeSessions: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GeneralSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoResumeSessions = source["autoResumeSessions"];
	    }
	}
	export class Notification {
	    id: string;
	    projectId: string;
	    type: string;
	    severity: string;
	    title: string;
	    payload?: Record<string, any>;
	    createdAt: number;
	    read: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Notification(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.type = source["type"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.payload = source["payload"];
	        this.createdAt = source["createdAt"];
	        this.read = source["read"];
	    }
	}
	export class NotificationEventFlags {
	    waiting: boolean;
	    completed: boolean;
	    errored: boolean;
	    started: boolean;
	    automation: boolean;
	    user: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NotificationEventFlags(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.waiting = source["waiting"];
	        this.completed = source["completed"];
	        this.errored = source["errored"];
	        this.started = source["started"];
	        this.automation = source["automation"];
	        this.user = source["user"];
	    }
	}
	export class NotificationSettings {
	    osEnabled: boolean;
	    sound: string;
	    silenceAll: boolean;
	    events: NotificationEventFlags;
	
	    static createFrom(source: any = {}) {
	        return new NotificationSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.osEnabled = source["osEnabled"];
	        this.sound = source["sound"];
	        this.silenceAll = source["silenceAll"];
	        this.events = this.convertValues(source["events"], NotificationEventFlags);
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
	export class Project {
	    id: string;
	    name: string;
	    color: string;
	    path: string;
	    logo: string;
	    integrations: Record<string, any>;
	    isolatedDefault: boolean;
	    branchPrefix: string;
	    hasGit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.path = source["path"];
	        this.logo = source["logo"];
	        this.integrations = source["integrations"];
	        this.isolatedDefault = source["isolatedDefault"];
	        this.branchPrefix = source["branchPrefix"];
	        this.hasGit = source["hasGit"];
	    }
	}
	export class ShortcutBinding {
	    key: string;
	    meta: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ShortcutBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.meta = source["meta"];
	    }
	}
	export class ShortcutsSettings {
	    openPalette: ShortcutBinding;
	    switchProject: ShortcutBinding;
	    addProject: ShortcutBinding;
	    newAgent: ShortcutBinding;
	    toggleSidebar: ShortcutBinding;
	    closeModal: ShortcutBinding;
	
	    static createFrom(source: any = {}) {
	        return new ShortcutsSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.openPalette = this.convertValues(source["openPalette"], ShortcutBinding);
	        this.switchProject = this.convertValues(source["switchProject"], ShortcutBinding);
	        this.addProject = this.convertValues(source["addProject"], ShortcutBinding);
	        this.newAgent = this.convertValues(source["newAgent"], ShortcutBinding);
	        this.toggleSidebar = this.convertValues(source["toggleSidebar"], ShortcutBinding);
	        this.closeModal = this.convertValues(source["closeModal"], ShortcutBinding);
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
	export class SpawnAgentInput {
	    projectId: string;
	    kind: string;
	    task: string;
	    model?: string;
	    binary?: string;
	    source?: string;
	    issueKey?: string;
	    issueSummary?: string;
	    issueType?: string;
	    isolated?: boolean;
	    branchName?: string;
	
	    static createFrom(source: any = {}) {
	        return new SpawnAgentInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.kind = source["kind"];
	        this.task = source["task"];
	        this.model = source["model"];
	        this.binary = source["binary"];
	        this.source = source["source"];
	        this.issueKey = source["issueKey"];
	        this.issueSummary = source["issueSummary"];
	        this.issueType = source["issueType"];
	        this.isolated = source["isolated"];
	        this.branchName = source["branchName"];
	    }
	}
	export class UpdateInfo {
	    current: string;
	    latest: string;
	    hasUpdate: boolean;
	    htmlUrl: string;
	    releaseNotes?: string;
	    publishedAt?: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.hasUpdate = source["hasUpdate"];
	        this.htmlUrl = source["htmlUrl"];
	        this.releaseNotes = source["releaseNotes"];
	        this.publishedAt = source["publishedAt"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace terminal {
	
	export class Terminal {
	    id: string;
	    name: string;
	    binary?: string;
	    path?: string;
	    installed: boolean;
	    default?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Terminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.binary = source["binary"];
	        this.path = source["path"];
	        this.installed = source["installed"];
	        this.default = source["default"];
	    }
	}

}

