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

