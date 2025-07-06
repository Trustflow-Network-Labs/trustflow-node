export namespace main {
	
	export class AddWorkflowJobResponse {
	    workflow_jobs_ids: number[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AddWorkflowJobResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow_jobs_ids = source["workflow_jobs_ids"];
	        this.error = source["error"];
	    }
	}
	export class AddWorkflowResponse {
	    workflow_id: number;
	    workflow_jobs_ids: number[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AddWorkflowResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow_id = source["workflow_id"];
	        this.workflow_jobs_ids = source["workflow_jobs_ids"];
	        this.error = source["error"];
	    }
	}
	export class ListWorkflowsResponse {
	    workflows: node_types.Workflow[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ListWorkflowsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflows = this.convertValues(source["workflows"], node_types.Workflow);
	        this.error = source["error"];
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
	export class ServiceCardGUIProps {
	    x: number;
	    y: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceCardGUIProps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.error = source["error"];
	    }
	}
	export class WorkflowGUIProps {
	    snap_to_grid: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowGUIProps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snap_to_grid = source["snap_to_grid"];
	        this.error = source["error"];
	    }
	}

}

export namespace node_types {
	
	export class WorkflowJobBase {
	    node_id: string;
	    service_id: number;
	    job_id: number;
	    expected_job_outputs: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowJobBase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.node_id = source["node_id"];
	        this.service_id = source["service_id"];
	        this.job_id = source["job_id"];
	        this.expected_job_outputs = source["expected_job_outputs"];
	    }
	}
	export class WorkflowJob {
	    id: number;
	    workflow_id: number;
	    workflow_job_base: WorkflowJobBase;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflow_id = source["workflow_id"];
	        this.workflow_job_base = this.convertValues(source["workflow_job_base"], WorkflowJobBase);
	        this.status = source["status"];
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
	export class Workflow {
	    id: number;
	    name: string;
	    description: string;
	    jobs: WorkflowJob[];
	
	    static createFrom(source: any = {}) {
	        return new Workflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.jobs = this.convertValues(source["jobs"], WorkflowJob);
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

