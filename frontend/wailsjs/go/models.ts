export namespace core {
	
	export class Card {
	    PID: number;
	    Order: number;
	    Name: string;
	    State: string;
	    Label: string;
	    Since: string;
	    Cwd: string;
	    Activity: string;
	    InTmux: boolean;
	    Private: boolean;
	    Attached: boolean;
	    CanPrompt: boolean;
	    PromptReason: string;
	    BackgroundAgents: number;
	    BackgroundShells: number;
	
	    static createFrom(source: any = {}) {
	        return new Card(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PID = source["PID"];
	        this.Order = source["Order"];
	        this.Name = source["Name"];
	        this.State = source["State"];
	        this.Label = source["Label"];
	        this.Since = source["Since"];
	        this.Cwd = source["Cwd"];
	        this.Activity = source["Activity"];
	        this.InTmux = source["InTmux"];
	        this.Private = source["Private"];
	        this.Attached = source["Attached"];
	        this.CanPrompt = source["CanPrompt"];
	        this.PromptReason = source["PromptReason"];
	        this.BackgroundAgents = source["BackgroundAgents"];
	        this.BackgroundShells = source["BackgroundShells"];
	    }
	}
	export class FocusResult {
	    Mode: string;
	    Warning: string;
	
	    static createFrom(source: any = {}) {
	        return new FocusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mode = source["Mode"];
	        this.Warning = source["Warning"];
	    }
	}
	export class Screen {
	    Lines: string[];
	    Width: number;
	    Height: number;
	    Hash: string;
	
	    static createFrom(source: any = {}) {
	        return new Screen(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Lines = source["Lines"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.Hash = source["Hash"];
	    }
	}

}

