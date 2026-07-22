export namespace app {

	export class WebServerSettings {
	    enabled: boolean;
	    port: string;
	    useTls: boolean;
	    certDomain?: string;
	    certPath?: string;
	    keyPath?: string;
	    certificateDirectory: string;
	    configDirectory: string;
	    hasPassword: boolean;
	    url?: string;

	    static createFrom(source: any = {}) {
	        return new WebServerSettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.useTls = source["useTls"];
	        this.certDomain = source["certDomain"];
	        this.certPath = source["certPath"];
	        this.keyPath = source["keyPath"];
	        this.certificateDirectory = source["certificateDirectory"];
	        this.configDirectory = source["configDirectory"];
	        this.hasPassword = source["hasPassword"];
	        this.url = source["url"];
	    }
	}
	export class WebServerSettingsInput {
	    enabled: boolean;
	    port: string;
	    password?: string;
	    useTls: boolean;
	    certDomain?: string;
	    certPath?: string;
	    keyPath?: string;

	    static createFrom(source: any = {}) {
	        return new WebServerSettingsInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.password = source["password"];
	        this.useTls = source["useTls"];
	        this.certDomain = source["certDomain"];
	        this.certPath = source["certPath"];
	        this.keyPath = source["keyPath"];
	    }
	}

}

export namespace llm {

	export class ModelInfo {
	    id: string;
	    displayName?: string;
	    supportsReasoning: boolean;
	    reasoningOptions?: string[];

	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	        this.supportsReasoning = source["supportsReasoning"];
	        this.reasoningOptions = source["reasoningOptions"];
	    }
	}
	export class ProviderSettings {
	    mode: string;
	    endpoint: string;
	    apiKey: string;
	    model: string;
	    reasoning?: string;
	    temperature?: number;
	    forceShowTemperature: boolean;
	    forceShowReasoning: boolean;
	    enablePostEdit: boolean;
	    enableTopicAwarePostEdit: boolean;
	    enableEnhancedContextTranslation: boolean;
	    enhancedContextGlossary?: string;
	    enableSmartChunking: boolean;
	    smartChunkSize?: number;
	    debugTranslationPromptTemplate?: string;
	    debugPostEditPromptTemplate?: string;

	    static createFrom(source: any = {}) {
	        return new ProviderSettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.endpoint = source["endpoint"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.reasoning = source["reasoning"];
	        this.temperature = source["temperature"];
	        this.forceShowTemperature = source["forceShowTemperature"];
	        this.forceShowReasoning = source["forceShowReasoning"];
	        this.enablePostEdit = source["enablePostEdit"];
	        this.enableTopicAwarePostEdit = source["enableTopicAwarePostEdit"];
	        this.enableEnhancedContextTranslation = source["enableEnhancedContextTranslation"];
	        this.enhancedContextGlossary = source["enhancedContextGlossary"];
	        this.enableSmartChunking = source["enableSmartChunking"];
	        this.smartChunkSize = source["smartChunkSize"];
	        this.debugTranslationPromptTemplate = source["debugTranslationPromptTemplate"];
	        this.debugPostEditPromptTemplate = source["debugPostEditPromptTemplate"];
	    }
	}
	export class TranslationRequest {
	    settings: ProviderSettings;
	    sourceText: string;
	    sourceLang: string;
	    targetLang: string;
	    instruction: string;
	    documentType?: string;

	    static createFrom(source: any = {}) {
	        return new TranslationRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], ProviderSettings);
	        this.sourceText = source["sourceText"];
	        this.sourceLang = source["sourceLang"];
	        this.targetLang = source["targetLang"];
	        this.instruction = source["instruction"];
	        this.documentType = source["documentType"];
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

export namespace pdfengine {

	export class TextRegion {
	    x: number;
	    y: number;
	    width: number;
	    height: number;

	    static createFrom(source: any = {}) {
	        return new TextRegion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class TextBlock {
	    id: string;
	    pageNumber: number;
	    blockIndex: number;
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	    fontSize: number;
	    role: string;
	    text: string;
	    regions?: TextRegion[];

	    static createFrom(source: any = {}) {
	        return new TextBlock(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.pageNumber = source["pageNumber"];
	        this.blockIndex = source["blockIndex"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fontSize = source["fontSize"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.regions = this.convertValues(source["regions"], TextRegion);
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
	export class Page {
	    pageNumber: number;
	    width: number;
	    height: number;
	    text: string;
	    blocks: TextBlock[];

	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pageNumber = source["pageNumber"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.text = source["text"];
	        this.blocks = this.convertValues(source["blocks"], TextBlock);
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
	export class ComposeRequest {
	    sourceName: string;
	    sourceDataBase64: string;
	    pages: Page[];
	    translatedText: string;

	    static createFrom(source: any = {}) {
	        return new ComposeRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceName = source["sourceName"];
	        this.sourceDataBase64 = source["sourceDataBase64"];
	        this.pages = this.convertValues(source["pages"], Page);
	        this.translatedText = source["translatedText"];
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
	export class Document {
	    name: string;
	    dataBase64: string;
	    pageCount: number;
	    pages: Page[];
	    sourceText: string;
	    extractedLength: number;

	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dataBase64 = source["dataBase64"];
	        this.pageCount = source["pageCount"];
	        this.pages = this.convertValues(source["pages"], Page);
	        this.sourceText = source["sourceText"];
	        this.extractedLength = source["extractedLength"];
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

	export class Result {
	    name: string;
	    dataBase64: string;
	    pageCount: number;
	    warning?: string;

	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dataBase64 = source["dataBase64"];
	        this.pageCount = source["pageCount"];
	        this.warning = source["warning"];
	    }
	}


}

