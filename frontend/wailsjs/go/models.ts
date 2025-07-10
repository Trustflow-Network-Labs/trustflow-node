export namespace main {
	
	export class AddWorkflowJobInterfacePeerResponse {
	    workflow_job_interface_peer_id: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AddWorkflowJobInterfacePeerResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow_job_interface_peer_id = source["workflow_job_interface_peer_id"];
	        this.error = source["error"];
	    }
	}
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
	export class GetWorkflowJobResponse {
	    workflow_job: node_types.WorkflowJob;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GetWorkflowJobResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow_job = this.convertValues(source["workflow_job"], node_types.WorkflowJob);
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
	export class ServiceInterface {
	    description: string;
	    interface_type: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.interface_type = source["interface_type"];
	        this.path = source["path"];
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
	
	export class NullString {
	    String: string;
	    Valid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NullString(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.String = source["String"];
	        this.Valid = source["Valid"];
	    }
	}
	export class ServiceInterfacePeer {
	    peer_service_id: number;
	    peer_node_id: string;
	    peer_path: string;
	    peer_mount_function: string;
	    peer_duty: number;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInterfacePeer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peer_service_id = source["peer_service_id"];
	        this.peer_node_id = source["peer_node_id"];
	        this.peer_path = source["peer_path"];
	        this.peer_mount_function = source["peer_mount_function"];
	        this.peer_duty = source["peer_duty"];
	    }
	}
	export class ServiceInterface {
	    interface_id: number;
	    workflow_job_id: number;
	    service_id: number;
	    service_interface_peers: ServiceInterfacePeer[];
	    interface_type: string;
	    description: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interface_id = source["interface_id"];
	        this.workflow_job_id = source["workflow_job_id"];
	        this.service_id = source["service_id"];
	        this.service_interface_peers = this.convertValues(source["service_interface_peers"], ServiceInterfacePeer);
	        this.interface_type = source["interface_type"];
	        this.description = source["description"];
	        this.path = source["path"];
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
	
	export class ServiceResourcesWithPricing {
	    resource_group: string;
	    resource_name: string;
	    resource_unit: string;
	    resource_description: NullString;
	    price: number;
	    currency_name: string;
	    currency_symbol: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceResourcesWithPricing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resource_group = source["resource_group"];
	        this.resource_name = source["resource_name"];
	        this.resource_unit = source["resource_unit"];
	        this.resource_description = this.convertValues(source["resource_description"], NullString);
	        this.price = source["price"];
	        this.currency_name = source["currency_name"];
	        this.currency_symbol = source["currency_symbol"];
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
	export class WorkflowJobBase {
	    id: number;
	    workflow_id: number;
	    node_id: string;
	    service_id: number;
	    service_name: string;
	    service_description: string;
	    service_type: string;
	    job_id: number;
	    expected_job_outputs: string;
	    status: string;
	    service_interfaces: ServiceInterface[];
	    service_price_model: ServiceResourcesWithPricing[];
	
	    static createFrom(source: any = {}) {
	        return new WorkflowJobBase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflow_id = source["workflow_id"];
	        this.node_id = source["node_id"];
	        this.service_id = source["service_id"];
	        this.service_name = source["service_name"];
	        this.service_description = source["service_description"];
	        this.service_type = source["service_type"];
	        this.job_id = source["job_id"];
	        this.expected_job_outputs = source["expected_job_outputs"];
	        this.status = source["status"];
	        this.service_interfaces = this.convertValues(source["service_interfaces"], ServiceInterface);
	        this.service_price_model = this.convertValues(source["service_price_model"], ServiceResourcesWithPricing);
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
	export class WorkflowJob {
	    workflow_job_base: WorkflowJobBase;
	    entrypoint: string[];
	    commands: string[];
	    // Go type: time
	    last_seen: any;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow_job_base = this.convertValues(source["workflow_job_base"], WorkflowJobBase);
	        this.entrypoint = source["entrypoint"];
	        this.commands = source["commands"];
	        this.last_seen = this.convertValues(source["last_seen"], null);
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

