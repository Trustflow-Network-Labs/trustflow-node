// Cynhyrchwyd y ffeil hon yn awtomatig. PEIDIWCH Â MODIWL
// This file is automatically generated. DO NOT EDIT
import {main} from '../models';
import {node_types} from '../models';

export function AddWorkflow(arg1:string,arg2:string,arg3:string,arg4:number,arg5:string,arg6:string,arg7:string,arg8:Array<string>,arg9:Array<string>,arg10:Array<main.ServiceInterface>,arg11:Array<node_types.ServiceResourcesWithPricing>,arg12:string,arg13:number,arg14:string):Promise<main.AddWorkflowResponse>;

export function AddWorkflowJob(arg1:number,arg2:string,arg3:number,arg4:string,arg5:string,arg6:string,arg7:Array<string>,arg8:Array<string>,arg9:Array<main.ServiceInterface>,arg10:Array<node_types.ServiceResourcesWithPricing>,arg11:string,arg12:number,arg13:string):Promise<main.AddWorkflowJobResponse>;

export function AddWorkflowJobInterfacePeer(arg1:number,arg2:number,arg3:string,arg4:number,arg5:string,arg6:string):Promise<main.AddWorkflowJobInterfacePeerResponse>;

export function CheckAndInstallDependencies():Promise<void>;

export function FindServices(arg1:string,arg2:string):Promise<void>;

export function GetServiceCardGUIProps(arg1:number):Promise<main.ServiceCardGUIProps>;

export function GetWorkflowGUIProps(arg1:number):Promise<main.WorkflowGUIProps>;

export function GetWorkflowJob(arg1:number):Promise<main.GetWorkflowJobResponse>;

export function IsHostRunning():Promise<boolean>;

export function ListWorkflows(arg1:number,arg2:number):Promise<main.ListWorkflowsResponse>;

export function NotifyFrontendReady():Promise<void>;

export function RemoveWorkflow(arg1:number):Promise<void>;

export function RemoveWorkflowJob(arg1:number):Promise<void>;

export function RemoveWorkflowJobInterfacePeer(arg1:number,arg2:number):Promise<void>;

export function SetServiceCardGUIProps(arg1:number,arg2:number,arg3:number):Promise<void>;

export function SetUserConfirmation(arg1:boolean):Promise<void>;

export function SetWorkflowGUIProps(arg1:number,arg2:number):Promise<void>;

export function StartNode(arg1:number,arg2:boolean,arg3:boolean):Promise<void>;

export function StopNode():Promise<void>;

export function UpdateWorkflow(arg1:number,arg2:string,arg3:string):Promise<void>;
