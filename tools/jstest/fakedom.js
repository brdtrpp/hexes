// A do-nothing DOM so Ebitengine's js init runs under node for layout tests.
const mk = () => {
  const f = function () {}; f._c = {};
  return new Proxy(f, {
    get(t, k) {
      if (k === Symbol.toPrimitive) return () => 0;
      if (k === 'then') return undefined;
      if (k === 'length') return 0;
      if (!t._c) t._c = {};
      if (!(k in t._c)) t._c[k] = mk();
      return t._c[k];
    },
    set() { return true; },
    apply() { return mk(); },
    construct() { return mk(); },
    has() { return true; },
    getOwnPropertyDescriptor(t, k) {
      if (k === 'prototype') return Reflect.getOwnPropertyDescriptor(t, k);
      return { get: mk(), set: mk(), configurable: true, enumerable: true };
    },
  });
};
for (const n of ['window', 'document', 'screen', 'Document', 'HTMLCanvasElement', 'matchMedia', 'requestAnimationFrame']) globalThis[n] = mk();
