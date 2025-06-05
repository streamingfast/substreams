// src/decorators.ts
import { fromBinary, toBinary as toBinary2 } from "@bufbuild/protobuf";

// src/store.ts
import { toBinary } from "@bufbuild/protobuf";
var Store = class {
  constructor(storeInterface, schema) {
    this.storeInterface = storeInterface;
    this.schema = schema;
  }
  set(ordinal, key, value) {
    const out = toBinary(this.schema, value);
    this.storeInterface.set(ordinal, key, out);
  }
};

// src/decorators.ts
var handlerRegistry = /* @__PURE__ */ new Map();
var substreams;
((substreams2) => {
  let handlers;
  ((handlers2) => {
    function map(inputTypes, outputType) {
      return function(target, propertyKey, descriptor) {
        const handler = descriptor.value;
        const instance = new target.constructor();
        handlerRegistry.set(propertyKey, {
          type: "map",
          inputTypes,
          outputType,
          handler,
          instance
        });
      };
    }
    handlers2.map = map;
    function store(storeType) {
      return function(target, propertyKey, descriptor) {
        const handler = descriptor.value;
        const instance = new target.constructor();
        handlerRegistry.set(propertyKey, {
          type: "store",
          handler,
          instance,
          storeType
        });
      };
    }
    handlers2.store = store;
  })(handlers = substreams2.handlers || (substreams2.handlers = {}));
})(substreams || (substreams = {}));
function executeMapHandler(handlerName, inputBytes) {
  const registered = handlerRegistry.get(handlerName);
  if (!registered || registered.type !== "map") {
    throw new Error(`Map handler '${handlerName}' not found`);
  }
  const inputs = registered.inputTypes.map((type) => fromBinary(type, inputBytes));
  const result = registered.handler.call(registered.instance, ...inputs);
  return toBinary2(registered.outputType, result);
}
function executeStoreHandler(handlerName, storeInterface, inputBytes) {
  const registered = handlerRegistry.get(handlerName);
  if (!registered || registered.type !== "store") {
    throw new Error(`Store handler '${handlerName}' not found`);
  }
  const store = new Store(storeInterface, registered.storeType);
  const input = fromBinary(registered.storeType, inputBytes);
  registered.handler.call(registered.instance, store, input);
}
function getHandlerType(handlerName) {
  return handlerRegistry.get(handlerName)?.type;
}
globalThis.getHandlerType = getHandlerType;
globalThis.executeMapHandler = executeMapHandler;
globalThis.executeStoreHandler = executeStoreHandler;

// src/utils.ts
Object.defineProperty(globalThis, "Promise", {
  configurable: false,
  enumerable: false,
  get() {
    throw new Error("Promises are disabled in this runtime");
  },
  set(_) {
    throw new Error("Cannot redefine Promise");
  }
});
function bytesToHex(arr) {
  return Array.from(arr, (b) => b.toString(16).padStart(2, "0")).join("");
}
function hexToBytes(hex) {
  if (hex.startsWith("0x"))
    hex = hex.slice(2);
  if (hex.length % 2)
    hex = "0" + hex;
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return out;
}
function safeParseLog(log, iface) {
  try {
    return iface.parseLog(log);
  } catch {
    return null;
  }
}
export {
  Store,
  bytesToHex,
  executeMapHandler,
  executeStoreHandler,
  getHandlerType,
  hexToBytes,
  safeParseLog,
  substreams
};
//# sourceMappingURL=index.mjs.map