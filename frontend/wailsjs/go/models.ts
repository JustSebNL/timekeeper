export namespace desktop {

	export class Health {
	    status: string;

	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	    }
	}
	export class TaskChildrenSummary {
	    subtask_ids: string[];
	    sprint_ids: string[];
	    open_subtasks: number;
	    open_sprints: number;
	    can_complete: boolean;

	    static createFrom(source: any = {}) {
	        return new TaskChildrenSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subtask_ids = source["subtask_ids"];
	        this.sprint_ids = source["sprint_ids"];
	        this.open_subtasks = source["open_subtasks"];
	        this.open_sprints = source["open_sprints"];
	        this.can_complete = source["can_complete"];
	    }
	}
	export class TaskDetail {
	    task: model.Task;
	    subtasks: model.Subtask[];
	    sprints: model.Sprint[];
	    children: TaskChildrenSummary;

	    static createFrom(source: any = {}) {
	        return new TaskDetail(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = this.convertValues(source["task"], model.Task);
	        this.subtasks = this.convertValues(source["subtasks"], model.Subtask);
	        this.sprints = this.convertValues(source["sprints"], model.Sprint);
	        this.children = this.convertValues(source["children"], TaskChildrenSummary);
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
	export class Version {
	    service: string;
	    version: string;
	    commit: string;
	    build_date: string;
	    go_version: string;

	    static createFrom(source: any = {}) {
	        return new Version(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = source["service"];
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.build_date = source["build_date"];
	        this.go_version = source["go_version"];
	    }
	}

}

export namespace main {

	export class DesktopStatus {
	    state: string;
	    message: string;
	    owned: boolean;

	    static createFrom(source: any = {}) {
	        return new DesktopStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.message = source["message"];
	        this.owned = source["owned"];
	    }
	}
	export class DialogFilter {
	    displayName: string;
	    pattern: string;

	    static createFrom(source: any = {}) {
	        return new DialogFilter(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayName = source["displayName"];
	        this.pattern = source["pattern"];
	    }
	}
	export class DialogOptions {
	    title: string;
	    defaultDirectory: string;
	    defaultFilename: string;
	    filters: DialogFilter[];

	    static createFrom(source: any = {}) {
	        return new DialogOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.defaultDirectory = source["defaultDirectory"];
	        this.defaultFilename = source["defaultFilename"];
	        this.filters = this.convertValues(source["filters"], DialogFilter);
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

export namespace model {

	export class Category {
	    category_id: string;
	    project_id: string;
	    parent_category_id?: string;
	    item_address: string;
	    name: string;
	    description?: string;
	    goal?: string;
	    status: string;
	    priority: string;
	    progress_pct: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category_id = source["category_id"];
	        this.project_id = source["project_id"];
	        this.parent_category_id = source["parent_category_id"];
	        this.item_address = source["item_address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.progress_pct = source["progress_pct"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class Subtask {
	    subtask_id: string;
	    project_id: string;
	    category_id: string;
	    task_id: string;
	    item_address: string;
	    name: string;
	    description?: string;
	    goal?: string;
	    status: string;
	    priority: string;
	    estimated_duration_seconds: number;
	    reported_completion_pct: number;
	    calculated_completion_pct: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Subtask(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subtask_id = source["subtask_id"];
	        this.project_id = source["project_id"];
	        this.category_id = source["category_id"];
	        this.task_id = source["task_id"];
	        this.item_address = source["item_address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.estimated_duration_seconds = source["estimated_duration_seconds"];
	        this.reported_completion_pct = source["reported_completion_pct"];
	        this.calculated_completion_pct = source["calculated_completion_pct"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class ExecutionSubtask {
	    subtask: Subtask;
	    sprints: Sprint[];

	    static createFrom(source: any = {}) {
	        return new ExecutionSubtask(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subtask = this.convertValues(source["subtask"], Subtask);
	        this.sprints = this.convertValues(source["sprints"], Sprint);
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
	export class Sprint {
	    sprint_id: string;
	    project_id: string;
	    category_id: string;
	    task_id: string;
	    subtask_id?: string;
	    item_address: string;
	    name: string;
	    description?: string;
	    goal?: string;
	    status: string;
	    priority: string;
	    estimated_duration_seconds: number;
	    buffer_pct: number;
	    buffer_duration_seconds: number;
	    active_duration_seconds: number;
	    hold_duration_seconds: number;
	    hold_reason?: string;
	    // Go type: time
	    started_at?: any;
	    // Go type: time
	    ended_at?: any;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Sprint(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sprint_id = source["sprint_id"];
	        this.project_id = source["project_id"];
	        this.category_id = source["category_id"];
	        this.task_id = source["task_id"];
	        this.subtask_id = source["subtask_id"];
	        this.item_address = source["item_address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.estimated_duration_seconds = source["estimated_duration_seconds"];
	        this.buffer_pct = source["buffer_pct"];
	        this.buffer_duration_seconds = source["buffer_duration_seconds"];
	        this.active_duration_seconds = source["active_duration_seconds"];
	        this.hold_duration_seconds = source["hold_duration_seconds"];
	        this.hold_reason = source["hold_reason"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.ended_at = this.convertValues(source["ended_at"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class Task {
	    task_id: string;
	    project_id: string;
	    category_id: string;
	    item_address: string;
	    name: string;
	    description?: string;
	    goal?: string;
	    status: string;
	    priority: string;
	    estimated_duration_seconds: number;
	    reported_completion_pct: number;
	    calculated_completion_pct: number;
	    auto_close: boolean;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.project_id = source["project_id"];
	        this.category_id = source["category_id"];
	        this.item_address = source["item_address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.estimated_duration_seconds = source["estimated_duration_seconds"];
	        this.reported_completion_pct = source["reported_completion_pct"];
	        this.calculated_completion_pct = source["calculated_completion_pct"];
	        this.auto_close = source["auto_close"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class ExecutionTask {
	    task: Task;
	    sprints: Sprint[];
	    subtasks: ExecutionSubtask[];

	    static createFrom(source: any = {}) {
	        return new ExecutionTask(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = this.convertValues(source["task"], Task);
	        this.sprints = this.convertValues(source["sprints"], Sprint);
	        this.subtasks = this.convertValues(source["subtasks"], ExecutionSubtask);
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
	export class ExecutionCategory {
	    category: Category;
	    categories: ExecutionCategory[];
	    tasks: ExecutionTask[];

	    static createFrom(source: any = {}) {
	        return new ExecutionCategory(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = this.convertValues(source["category"], Category);
	        this.categories = this.convertValues(source["categories"], ExecutionCategory);
	        this.tasks = this.convertValues(source["tasks"], ExecutionTask);
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
	    project_id: string;
	    project_number: number;
	    item_address: string;
	    project_name: string;
	    project_description?: string;
	    project_goal?: string;
	    planned_start_date?: string;
	    target_end_date?: string;
	    // Go type: time
	    actual_started_at?: any;
	    // Go type: time
	    completed_at?: any;
	    // Go type: time
	    cancelled_at?: any;
	    lifecycle_phase: string;
	    schedule_timezone: string;
	    deadline_warning_days: number;
	    deadline_state?: string;
	    days_to_deadline?: number;
	    status: string;
	    priority: string;
	    palette_id: number;
	    reported_completion_pct: number;
	    calculated_completion_pct: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    project_alias?: string;

	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.project_number = source["project_number"];
	        this.item_address = source["item_address"];
	        this.project_name = source["project_name"];
	        this.project_description = source["project_description"];
	        this.project_goal = source["project_goal"];
	        this.planned_start_date = source["planned_start_date"];
	        this.target_end_date = source["target_end_date"];
	        this.actual_started_at = this.convertValues(source["actual_started_at"], null);
	        this.completed_at = this.convertValues(source["completed_at"], null);
	        this.cancelled_at = this.convertValues(source["cancelled_at"], null);
	        this.lifecycle_phase = source["lifecycle_phase"];
	        this.schedule_timezone = source["schedule_timezone"];
	        this.deadline_warning_days = source["deadline_warning_days"];
	        this.deadline_state = source["deadline_state"];
	        this.days_to_deadline = source["days_to_deadline"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.palette_id = source["palette_id"];
	        this.reported_completion_pct = source["reported_completion_pct"];
	        this.calculated_completion_pct = source["calculated_completion_pct"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.project_alias = source["project_alias"];
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
	export class ExecutionTree {
	    project: Project;
	    categories: ExecutionCategory[];

	    static createFrom(source: any = {}) {
	        return new ExecutionTree(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], Project);
	        this.categories = this.convertValues(source["categories"], ExecutionCategory);
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
