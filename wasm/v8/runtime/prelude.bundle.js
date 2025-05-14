(() => {
  var __create = Object.create;
  var __defProp = Object.defineProperty;
  var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
  var __getOwnPropNames = Object.getOwnPropertyNames;
  var __getProtoOf = Object.getPrototypeOf;
  var __hasOwnProp = Object.prototype.hasOwnProperty;
  var __knownSymbol = (name, symbol2) => (symbol2 = Symbol[name]) ? symbol2 : Symbol.for("Symbol." + name);
  var __typeError = (msg) => {
    throw TypeError(msg);
  };
  var __commonJS = (cb, mod) => function __require() {
    return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
  };
  var __copyProps = (to, from, except, desc) => {
    if (from && typeof from === "object" || typeof from === "function") {
      for (let key of __getOwnPropNames(from))
        if (!__hasOwnProp.call(to, key) && key !== except)
          __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    }
    return to;
  };
  var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
    // If the importer is in node compatibility mode or this is not an ESM
    // file that has been converted to a CommonJS file using a Babel-
    // compatible transform (i.e. "__esModule" has not been set), then set
    // "default" to the CommonJS "module.exports" for node compatibility.
    isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: true }) : target,
    mod
  ));
  var __await = function(promise, isYieldStar) {
    this[0] = promise;
    this[1] = isYieldStar;
  };
  var __yieldStar = (value) => {
    var obj = value[__knownSymbol("asyncIterator")], isAwait = false, method, it = {};
    if (obj == null) {
      obj = value[__knownSymbol("iterator")]();
      method = (k) => it[k] = (x) => obj[k](x);
    } else {
      obj = obj.call(value);
      method = (k) => it[k] = (v) => {
        if (isAwait) {
          isAwait = false;
          if (k === "throw") throw v;
          return v;
        }
        isAwait = true;
        return {
          done: false,
          value: new __await(new Promise((resolve) => {
            var x = obj[k](v);
            if (!(x instanceof Object)) __typeError("Object expected");
            resolve(x);
          }), 1)
        };
      };
    }
    return it[__knownSymbol("iterator")] = () => it, method("next"), "throw" in obj ? method("throw") : it.throw = (x) => {
      throw x;
    }, "return" in obj && method("return"), it;
  };

  // ../bench/substreams_ts/shims/bigInt/index.js
  var require_bigInt = __commonJS({
    "../bench/substreams_ts/shims/bigInt/index.js"(exports, module) {
      "use strict";
      var bigInt2 = function(t) {
        "use strict";
        var e = 1e7, r = 9007199254740992, o = f(r), n = "0123456789abcdefghijklmnopqrstuvwxyz", i = "function" == typeof BigInt;
        function u(t2, e2, r2, o2) {
          return void 0 === t2 ? u[0] : void 0 !== e2 && (10 != +e2 || r2) ? _(t2, e2, r2, o2) : K(t2);
        }
        function p(t2, e2) {
          this.value = t2, this.sign = e2, this.isSmall = false;
        }
        function a(t2) {
          this.value = t2, this.sign = t2 < 0, this.isSmall = true;
        }
        function s(t2) {
          this.value = t2;
        }
        function l(t2) {
          return -r < t2 && t2 < r;
        }
        function f(t2) {
          return t2 < 1e7 ? [t2] : t2 < 1e14 ? [t2 % 1e7, Math.floor(t2 / 1e7)] : [t2 % 1e7, Math.floor(t2 / 1e7) % 1e7, Math.floor(t2 / 1e14)];
        }
        function v(t2) {
          h(t2);
          var r2 = t2.length;
          if (r2 < 4 && A(t2, o) < 0) switch (r2) {
            case 0:
              return 0;
            case 1:
              return t2[0];
            case 2:
              return t2[0] + t2[1] * e;
            default:
              return t2[0] + (t2[1] + t2[2] * e) * e;
          }
          return t2;
        }
        function h(t2) {
          for (var e2 = t2.length; 0 === t2[--e2]; ) ;
          t2.length = e2 + 1;
        }
        function y(t2) {
          for (var e2 = new Array(t2), r2 = -1; ++r2 < t2; ) e2[r2] = 0;
          return e2;
        }
        function g(t2) {
          return t2 > 0 ? Math.floor(t2) : Math.ceil(t2);
        }
        function c(t2, r2) {
          var o2, n2, i2 = t2.length, u2 = r2.length, p2 = new Array(i2), a2 = 0, s2 = e;
          for (n2 = 0; n2 < u2; n2++) a2 = (o2 = t2[n2] + r2[n2] + a2) >= s2 ? 1 : 0, p2[n2] = o2 - a2 * s2;
          for (; n2 < i2; ) a2 = (o2 = t2[n2] + a2) === s2 ? 1 : 0, p2[n2++] = o2 - a2 * s2;
          return a2 > 0 && p2.push(a2), p2;
        }
        function m(t2, e2) {
          return t2.length >= e2.length ? c(t2, e2) : c(e2, t2);
        }
        function d(t2, r2) {
          var o2, n2, i2 = t2.length, u2 = new Array(i2), p2 = e;
          for (n2 = 0; n2 < i2; n2++) o2 = t2[n2] - p2 + r2, r2 = Math.floor(o2 / p2), u2[n2] = o2 - r2 * p2, r2 += 1;
          for (; r2 > 0; ) u2[n2++] = r2 % p2, r2 = Math.floor(r2 / p2);
          return u2;
        }
        function b(t2, r2) {
          var o2, n2, i2 = t2.length, u2 = r2.length, p2 = new Array(i2), a2 = 0, s2 = e;
          for (o2 = 0; o2 < u2; o2++) (n2 = t2[o2] - a2 - r2[o2]) < 0 ? (n2 += s2, a2 = 1) : a2 = 0, p2[o2] = n2;
          for (o2 = u2; o2 < i2; o2++) {
            if (!((n2 = t2[o2] - a2) < 0)) {
              p2[o2++] = n2;
              break;
            }
            n2 += s2, p2[o2] = n2;
          }
          for (; o2 < i2; o2++) p2[o2] = t2[o2];
          return h(p2), p2;
        }
        function w(t2, r2, o2) {
          var n2, i2, u2 = t2.length, s2 = new Array(u2), l2 = -r2, f2 = e;
          for (n2 = 0; n2 < u2; n2++) i2 = t2[n2] + l2, l2 = Math.floor(i2 / f2), i2 %= f2, s2[n2] = i2 < 0 ? i2 + f2 : i2;
          return "number" == typeof (s2 = v(s2)) ? (o2 && (s2 = -s2), new a(s2)) : new p(s2, o2);
        }
        function S(t2, r2) {
          var o2, n2, i2, u2, p2 = t2.length, a2 = r2.length, s2 = y(p2 + a2), l2 = e;
          for (i2 = 0; i2 < p2; ++i2) {
            u2 = t2[i2];
            for (var f2 = 0; f2 < a2; ++f2) o2 = u2 * r2[f2] + s2[i2 + f2], n2 = Math.floor(o2 / l2), s2[i2 + f2] = o2 - n2 * l2, s2[i2 + f2 + 1] += n2;
          }
          return h(s2), s2;
        }
        function I(t2, r2) {
          var o2, n2, i2 = t2.length, u2 = new Array(i2), p2 = e, a2 = 0;
          for (n2 = 0; n2 < i2; n2++) o2 = t2[n2] * r2 + a2, a2 = Math.floor(o2 / p2), u2[n2] = o2 - a2 * p2;
          for (; a2 > 0; ) u2[n2++] = a2 % p2, a2 = Math.floor(a2 / p2);
          return u2;
        }
        function q(t2, e2) {
          for (var r2 = []; e2-- > 0; ) r2.push(0);
          return r2.concat(t2);
        }
        function M(t2, e2) {
          var r2 = Math.max(t2.length, e2.length);
          if (r2 <= 30) return S(t2, e2);
          r2 = Math.ceil(r2 / 2);
          var o2 = t2.slice(r2), n2 = t2.slice(0, r2), i2 = e2.slice(r2), u2 = e2.slice(0, r2), p2 = M(n2, u2), a2 = M(o2, i2), s2 = M(m(n2, o2), m(u2, i2)), l2 = m(m(p2, q(b(b(s2, p2), a2), r2)), q(a2, 2 * r2));
          return h(l2), l2;
        }
        function N(t2, r2, o2) {
          return new p(t2 < e ? I(r2, t2) : S(r2, f(t2)), o2);
        }
        function E(t2) {
          var r2, o2, n2, i2, u2 = t2.length, p2 = y(u2 + u2), a2 = e;
          for (n2 = 0; n2 < u2; n2++) {
            o2 = 0 - (i2 = t2[n2]) * i2;
            for (var s2 = n2; s2 < u2; s2++) r2 = i2 * t2[s2] * 2 + p2[n2 + s2] + o2, o2 = Math.floor(r2 / a2), p2[n2 + s2] = r2 - o2 * a2;
            p2[n2 + u2] = o2;
          }
          return h(p2), p2;
        }
        function O(t2, e2) {
          var r2, o2, n2, i2, u2 = t2.length, p2 = y(u2);
          for (n2 = 0, r2 = u2 - 1; r2 >= 0; --r2) n2 = (i2 = 1e7 * n2 + t2[r2]) - (o2 = g(i2 / e2)) * e2, p2[r2] = 0 | o2;
          return [p2, 0 | n2];
        }
        function B(t2, r2) {
          var o2, n2 = K(r2);
          if (i) return [new s(t2.value / n2.value), new s(t2.value % n2.value)];
          var l2, c2 = t2.value, m2 = n2.value;
          if (0 === m2) throw new Error("Cannot divide by zero");
          if (t2.isSmall) return n2.isSmall ? [new a(g(c2 / m2)), new a(c2 % m2)] : [u[0], t2];
          if (n2.isSmall) {
            if (1 === m2) return [t2, u[0]];
            if (-1 == m2) return [t2.negate(), u[0]];
            var d2 = Math.abs(m2);
            if (d2 < e) {
              l2 = v((o2 = O(c2, d2))[0]);
              var w2 = o2[1];
              return t2.sign && (w2 = -w2), "number" == typeof l2 ? (t2.sign !== n2.sign && (l2 = -l2), [new a(l2), new a(w2)]) : [new p(l2, t2.sign !== n2.sign), new a(w2)];
            }
            m2 = f(d2);
          }
          var S2 = A(c2, m2);
          if (-1 === S2) return [u[0], t2];
          if (0 === S2) return [u[t2.sign === n2.sign ? 1 : -1], u[0]];
          o2 = c2.length + m2.length <= 200 ? function(t3, r3) {
            var o3, n3, i2, u2, p2, a2, s2, l3 = t3.length, f2 = r3.length, h2 = e, g2 = y(r3.length), c3 = r3[f2 - 1], m3 = Math.ceil(h2 / (2 * c3)), d3 = I(t3, m3), b2 = I(r3, m3);
            for (d3.length <= l3 && d3.push(0), b2.push(0), c3 = b2[f2 - 1], n3 = l3 - f2; n3 >= 0; n3--) {
              for (o3 = h2 - 1, d3[n3 + f2] !== c3 && (o3 = Math.floor((d3[n3 + f2] * h2 + d3[n3 + f2 - 1]) / c3)), i2 = 0, u2 = 0, a2 = b2.length, p2 = 0; p2 < a2; p2++) i2 += o3 * b2[p2], s2 = Math.floor(i2 / h2), u2 += d3[n3 + p2] - (i2 - s2 * h2), i2 = s2, u2 < 0 ? (d3[n3 + p2] = u2 + h2, u2 = -1) : (d3[n3 + p2] = u2, u2 = 0);
              for (; 0 !== u2; ) {
                for (o3 -= 1, i2 = 0, p2 = 0; p2 < a2; p2++) (i2 += d3[n3 + p2] - h2 + b2[p2]) < 0 ? (d3[n3 + p2] = i2 + h2, i2 = 0) : (d3[n3 + p2] = i2, i2 = 1);
                u2 += i2;
              }
              g2[n3] = o3;
            }
            return d3 = O(d3, m3)[0], [v(g2), v(d3)];
          }(c2, m2) : function(t3, r3) {
            for (var o3, n3, i2, u2, p2, a2 = t3.length, s2 = r3.length, l3 = [], f2 = [], y2 = e; a2; ) if (f2.unshift(t3[--a2]), h(f2), A(f2, r3) < 0) l3.push(0);
            else {
              i2 = f2[(n3 = f2.length) - 1] * y2 + f2[n3 - 2], u2 = r3[s2 - 1] * y2 + r3[s2 - 2], n3 > s2 && (i2 = (i2 + 1) * y2), o3 = Math.ceil(i2 / u2);
              do {
                if (A(p2 = I(r3, o3), f2) <= 0) break;
                o3--;
              } while (o3);
              l3.push(o3), f2 = b(f2, p2);
            }
            return l3.reverse(), [v(l3), v(f2)];
          }(c2, m2), l2 = o2[0];
          var q2 = t2.sign !== n2.sign, M2 = o2[1], N2 = t2.sign;
          return "number" == typeof l2 ? (q2 && (l2 = -l2), l2 = new a(l2)) : l2 = new p(l2, q2), "number" == typeof M2 ? (N2 && (M2 = -M2), M2 = new a(M2)) : M2 = new p(M2, N2), [l2, M2];
        }
        function A(t2, e2) {
          if (t2.length !== e2.length) return t2.length > e2.length ? 1 : -1;
          for (var r2 = t2.length - 1; r2 >= 0; r2--) if (t2[r2] !== e2[r2]) return t2[r2] > e2[r2] ? 1 : -1;
          return 0;
        }
        function P(t2) {
          var e2 = t2.abs();
          return !e2.isUnit() && (!!(e2.equals(2) || e2.equals(3) || e2.equals(5)) || !(e2.isEven() || e2.isDivisibleBy(3) || e2.isDivisibleBy(5)) && (!!e2.lesser(49) || void 0));
        }
        function Z(t2, e2) {
          for (var r2, o2, n2, i2 = t2.prev(), u2 = i2, p2 = 0; u2.isEven(); ) u2 = u2.divide(2), p2++;
          t: for (o2 = 0; o2 < e2.length; o2++) if (!t2.lesser(e2[o2]) && !(n2 = bigInt2(e2[o2]).modPow(u2, t2)).isUnit() && !n2.equals(i2)) {
            for (r2 = p2 - 1; 0 != r2; r2--) {
              if ((n2 = n2.square().mod(t2)).isUnit()) return false;
              if (n2.equals(i2)) continue t;
            }
            return false;
          }
          return true;
        }
        p.prototype = Object.create(u.prototype), a.prototype = Object.create(u.prototype), s.prototype = Object.create(u.prototype), p.prototype.add = function(t2) {
          var e2 = K(t2);
          if (this.sign !== e2.sign) return this.subtract(e2.negate());
          var r2 = this.value, o2 = e2.value;
          return e2.isSmall ? new p(d(r2, Math.abs(o2)), this.sign) : new p(m(r2, o2), this.sign);
        }, p.prototype.plus = p.prototype.add, a.prototype.add = function(t2) {
          var e2 = K(t2), r2 = this.value;
          if (r2 < 0 !== e2.sign) return this.subtract(e2.negate());
          var o2 = e2.value;
          if (e2.isSmall) {
            if (l(r2 + o2)) return new a(r2 + o2);
            o2 = f(Math.abs(o2));
          }
          return new p(d(o2, Math.abs(r2)), r2 < 0);
        }, a.prototype.plus = a.prototype.add, s.prototype.add = function(t2) {
          return new s(this.value + K(t2).value);
        }, s.prototype.plus = s.prototype.add, p.prototype.subtract = function(t2) {
          var e2 = K(t2);
          if (this.sign !== e2.sign) return this.add(e2.negate());
          var r2 = this.value, o2 = e2.value;
          return e2.isSmall ? w(r2, Math.abs(o2), this.sign) : function(t3, e3, r3) {
            var o3;
            return A(t3, e3) >= 0 ? o3 = b(t3, e3) : (o3 = b(e3, t3), r3 = !r3), "number" == typeof (o3 = v(o3)) ? (r3 && (o3 = -o3), new a(o3)) : new p(o3, r3);
          }(r2, o2, this.sign);
        }, p.prototype.minus = p.prototype.subtract, a.prototype.subtract = function(t2) {
          var e2 = K(t2), r2 = this.value;
          if (r2 < 0 !== e2.sign) return this.add(e2.negate());
          var o2 = e2.value;
          return e2.isSmall ? new a(r2 - o2) : w(o2, Math.abs(r2), r2 >= 0);
        }, a.prototype.minus = a.prototype.subtract, s.prototype.subtract = function(t2) {
          return new s(this.value - K(t2).value);
        }, s.prototype.minus = s.prototype.subtract, p.prototype.negate = function() {
          return new p(this.value, !this.sign);
        }, a.prototype.negate = function() {
          var t2 = this.sign, e2 = new a(-this.value);
          return e2.sign = !t2, e2;
        }, s.prototype.negate = function() {
          return new s(-this.value);
        }, p.prototype.abs = function() {
          return new p(this.value, false);
        }, a.prototype.abs = function() {
          return new a(Math.abs(this.value));
        }, s.prototype.abs = function() {
          return new s(this.value >= 0 ? this.value : -this.value);
        }, p.prototype.multiply = function(t2) {
          var r2, o2, n2, i2 = K(t2), a2 = this.value, s2 = i2.value, l2 = this.sign !== i2.sign;
          if (i2.isSmall) {
            if (0 === s2) return u[0];
            if (1 === s2) return this;
            if (-1 === s2) return this.negate();
            if ((r2 = Math.abs(s2)) < e) return new p(I(a2, r2), l2);
            s2 = f(r2);
          }
          return o2 = a2.length, n2 = s2.length, new p(-0.012 * o2 - 0.012 * n2 + 15e-6 * o2 * n2 > 0 ? M(a2, s2) : S(a2, s2), l2);
        }, p.prototype.times = p.prototype.multiply, a.prototype._multiplyBySmall = function(t2) {
          return l(t2.value * this.value) ? new a(t2.value * this.value) : N(Math.abs(t2.value), f(Math.abs(this.value)), this.sign !== t2.sign);
        }, p.prototype._multiplyBySmall = function(t2) {
          return 0 === t2.value ? u[0] : 1 === t2.value ? this : -1 === t2.value ? this.negate() : N(Math.abs(t2.value), this.value, this.sign !== t2.sign);
        }, a.prototype.multiply = function(t2) {
          return K(t2)._multiplyBySmall(this);
        }, a.prototype.times = a.prototype.multiply, s.prototype.multiply = function(t2) {
          return new s(this.value * K(t2).value);
        }, s.prototype.times = s.prototype.multiply, p.prototype.square = function() {
          return new p(E(this.value), false);
        }, a.prototype.square = function() {
          var t2 = this.value * this.value;
          return l(t2) ? new a(t2) : new p(E(f(Math.abs(this.value))), false);
        }, s.prototype.square = function(t2) {
          return new s(this.value * this.value);
        }, p.prototype.divmod = function(t2) {
          var e2 = B(this, t2);
          return { quotient: e2[0], remainder: e2[1] };
        }, s.prototype.divmod = a.prototype.divmod = p.prototype.divmod, p.prototype.divide = function(t2) {
          return B(this, t2)[0];
        }, s.prototype.over = s.prototype.divide = function(t2) {
          return new s(this.value / K(t2).value);
        }, a.prototype.over = a.prototype.divide = p.prototype.over = p.prototype.divide, p.prototype.mod = function(t2) {
          return B(this, t2)[1];
        }, s.prototype.mod = s.prototype.remainder = function(t2) {
          return new s(this.value % K(t2).value);
        }, a.prototype.remainder = a.prototype.mod = p.prototype.remainder = p.prototype.mod, p.prototype.pow = function(t2) {
          var e2, r2, o2, n2 = K(t2), i2 = this.value, p2 = n2.value;
          if (0 === p2) return u[1];
          if (0 === i2) return u[0];
          if (1 === i2) return u[1];
          if (-1 === i2) return n2.isEven() ? u[1] : u[-1];
          if (n2.sign) return u[0];
          if (!n2.isSmall) throw new Error("The exponent " + n2.toString() + " is too large.");
          if (this.isSmall && l(e2 = Math.pow(i2, p2))) return new a(g(e2));
          for (r2 = this, o2 = u[1]; true & p2 && (o2 = o2.times(r2), --p2), 0 !== p2; ) p2 /= 2, r2 = r2.square();
          return o2;
        }, a.prototype.pow = p.prototype.pow, s.prototype.pow = function(t2) {
          var e2 = K(t2), r2 = this.value, o2 = e2.value, n2 = BigInt(0), i2 = BigInt(1), p2 = BigInt(2);
          if (o2 === n2) return u[1];
          if (r2 === n2) return u[0];
          if (r2 === i2) return u[1];
          if (r2 === BigInt(-1)) return e2.isEven() ? u[1] : u[-1];
          if (e2.isNegative()) return new s(n2);
          for (var a2 = this, l2 = u[1]; (o2 & i2) === i2 && (l2 = l2.times(a2), --o2), o2 !== n2; ) o2 /= p2, a2 = a2.square();
          return l2;
        }, p.prototype.modPow = function(t2, e2) {
          if (t2 = K(t2), (e2 = K(e2)).isZero()) throw new Error("Cannot take modPow with modulus 0");
          var r2 = u[1], o2 = this.mod(e2);
          for (t2.isNegative() && (t2 = t2.multiply(u[-1]), o2 = o2.modInv(e2)); t2.isPositive(); ) {
            if (o2.isZero()) return u[0];
            t2.isOdd() && (r2 = r2.multiply(o2).mod(e2)), t2 = t2.divide(2), o2 = o2.square().mod(e2);
          }
          return r2;
        }, s.prototype.modPow = a.prototype.modPow = p.prototype.modPow, p.prototype.compareAbs = function(t2) {
          var e2 = K(t2), r2 = this.value, o2 = e2.value;
          return e2.isSmall ? 1 : A(r2, o2);
        }, a.prototype.compareAbs = function(t2) {
          var e2 = K(t2), r2 = Math.abs(this.value), o2 = e2.value;
          return e2.isSmall ? r2 === (o2 = Math.abs(o2)) ? 0 : r2 > o2 ? 1 : -1 : -1;
        }, s.prototype.compareAbs = function(t2) {
          var e2 = this.value, r2 = K(t2).value;
          return (e2 = e2 >= 0 ? e2 : -e2) === (r2 = r2 >= 0 ? r2 : -r2) ? 0 : e2 > r2 ? 1 : -1;
        }, p.prototype.compare = function(t2) {
          if (t2 === 1 / 0) return -1;
          if (t2 === -1 / 0) return 1;
          var e2 = K(t2), r2 = this.value, o2 = e2.value;
          return this.sign !== e2.sign ? e2.sign ? 1 : -1 : e2.isSmall ? this.sign ? -1 : 1 : A(r2, o2) * (this.sign ? -1 : 1);
        }, p.prototype.compareTo = p.prototype.compare, a.prototype.compare = function(t2) {
          if (t2 === 1 / 0) return -1;
          if (t2 === -1 / 0) return 1;
          var e2 = K(t2), r2 = this.value, o2 = e2.value;
          return e2.isSmall ? r2 == o2 ? 0 : r2 > o2 ? 1 : -1 : r2 < 0 !== e2.sign ? r2 < 0 ? -1 : 1 : r2 < 0 ? 1 : -1;
        }, a.prototype.compareTo = a.prototype.compare, s.prototype.compare = function(t2) {
          if (t2 === 1 / 0) return -1;
          if (t2 === -1 / 0) return 1;
          var e2 = this.value, r2 = K(t2).value;
          return e2 === r2 ? 0 : e2 > r2 ? 1 : -1;
        }, s.prototype.compareTo = s.prototype.compare, p.prototype.equals = function(t2) {
          return 0 === this.compare(t2);
        }, s.prototype.eq = s.prototype.equals = a.prototype.eq = a.prototype.equals = p.prototype.eq = p.prototype.equals, p.prototype.notEquals = function(t2) {
          return 0 !== this.compare(t2);
        }, s.prototype.neq = s.prototype.notEquals = a.prototype.neq = a.prototype.notEquals = p.prototype.neq = p.prototype.notEquals, p.prototype.greater = function(t2) {
          return this.compare(t2) > 0;
        }, s.prototype.gt = s.prototype.greater = a.prototype.gt = a.prototype.greater = p.prototype.gt = p.prototype.greater, p.prototype.lesser = function(t2) {
          return this.compare(t2) < 0;
        }, s.prototype.lt = s.prototype.lesser = a.prototype.lt = a.prototype.lesser = p.prototype.lt = p.prototype.lesser, p.prototype.greaterOrEquals = function(t2) {
          return this.compare(t2) >= 0;
        }, s.prototype.geq = s.prototype.greaterOrEquals = a.prototype.geq = a.prototype.greaterOrEquals = p.prototype.geq = p.prototype.greaterOrEquals, p.prototype.lesserOrEquals = function(t2) {
          return this.compare(t2) <= 0;
        }, s.prototype.leq = s.prototype.lesserOrEquals = a.prototype.leq = a.prototype.lesserOrEquals = p.prototype.leq = p.prototype.lesserOrEquals, p.prototype.isEven = function() {
          return 0 == (1 & this.value[0]);
        }, a.prototype.isEven = function() {
          return 0 == (1 & this.value);
        }, s.prototype.isEven = function() {
          return (this.value & BigInt(1)) === BigInt(0);
        }, p.prototype.isOdd = function() {
          return 1 == (1 & this.value[0]);
        }, a.prototype.isOdd = function() {
          return 1 == (1 & this.value);
        }, s.prototype.isOdd = function() {
          return (this.value & BigInt(1)) === BigInt(1);
        }, p.prototype.isPositive = function() {
          return !this.sign;
        }, a.prototype.isPositive = function() {
          return this.value > 0;
        }, s.prototype.isPositive = a.prototype.isPositive, p.prototype.isNegative = function() {
          return this.sign;
        }, a.prototype.isNegative = function() {
          return this.value < 0;
        }, s.prototype.isNegative = a.prototype.isNegative, p.prototype.isUnit = function() {
          return false;
        }, a.prototype.isUnit = function() {
          return 1 === Math.abs(this.value);
        }, s.prototype.isUnit = function() {
          return this.abs().value === BigInt(1);
        }, p.prototype.isZero = function() {
          return false;
        }, a.prototype.isZero = function() {
          return 0 === this.value;
        }, s.prototype.isZero = function() {
          return this.value === BigInt(0);
        }, p.prototype.isDivisibleBy = function(t2) {
          var e2 = K(t2);
          return !e2.isZero() && (!!e2.isUnit() || (0 === e2.compareAbs(2) ? this.isEven() : this.mod(e2).isZero()));
        }, s.prototype.isDivisibleBy = a.prototype.isDivisibleBy = p.prototype.isDivisibleBy, p.prototype.isPrime = function(e2) {
          var r2 = P(this);
          if (r2 !== t) return r2;
          var o2 = this.abs(), n2 = o2.bitLength();
          if (n2 <= 64) return Z(o2, [2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37]);
          for (var i2 = Math.log(2) * n2.toJSNumber(), u2 = Math.ceil(true === e2 ? 2 * Math.pow(i2, 2) : i2), p2 = [], a2 = 0; a2 < u2; a2++) p2.push(bigInt2(a2 + 2));
          return Z(o2, p2);
        }, s.prototype.isPrime = a.prototype.isPrime = p.prototype.isPrime, p.prototype.isProbablePrime = function(e2, r2) {
          var o2 = P(this);
          if (o2 !== t) return o2;
          for (var n2 = this.abs(), i2 = e2 === t ? 5 : e2, u2 = [], p2 = 0; p2 < i2; p2++) u2.push(bigInt2.randBetween(2, n2.minus(2), r2));
          return Z(n2, u2);
        }, s.prototype.isProbablePrime = a.prototype.isProbablePrime = p.prototype.isProbablePrime, p.prototype.modInv = function(t2) {
          for (var e2, r2, o2, n2 = bigInt2.zero, i2 = bigInt2.one, u2 = K(t2), p2 = this.abs(); !p2.isZero(); ) e2 = u2.divide(p2), r2 = n2, o2 = u2, n2 = i2, u2 = p2, i2 = r2.subtract(e2.multiply(i2)), p2 = o2.subtract(e2.multiply(p2));
          if (!u2.isUnit()) throw new Error(this.toString() + " and " + t2.toString() + " are not co-prime");
          return -1 === n2.compare(0) && (n2 = n2.add(t2)), this.isNegative() ? n2.negate() : n2;
        }, s.prototype.modInv = a.prototype.modInv = p.prototype.modInv, p.prototype.next = function() {
          var t2 = this.value;
          return this.sign ? w(t2, 1, this.sign) : new p(d(t2, 1), this.sign);
        }, a.prototype.next = function() {
          var t2 = this.value;
          return t2 + 1 < r ? new a(t2 + 1) : new p(o, false);
        }, s.prototype.next = function() {
          return new s(this.value + BigInt(1));
        }, p.prototype.prev = function() {
          var t2 = this.value;
          return this.sign ? new p(d(t2, 1), true) : w(t2, 1, this.sign);
        }, a.prototype.prev = function() {
          var t2 = this.value;
          return t2 - 1 > -r ? new a(t2 - 1) : new p(o, true);
        }, s.prototype.prev = function() {
          return new s(this.value - BigInt(1));
        };
        for (var x = [1]; 2 * x[x.length - 1] <= e; ) x.push(2 * x[x.length - 1]);
        var J = x.length, L = x[J - 1];
        function U(t2) {
          return Math.abs(t2) <= e;
        }
        function T(t2, e2, r2) {
          e2 = K(e2);
          for (var o2 = t2.isNegative(), n2 = e2.isNegative(), i2 = o2 ? t2.not() : t2, u2 = n2 ? e2.not() : e2, p2 = 0, a2 = 0, s2 = null, l2 = null, f2 = []; !i2.isZero() || !u2.isZero(); ) p2 = (s2 = B(i2, L))[1].toJSNumber(), o2 && (p2 = L - 1 - p2), a2 = (l2 = B(u2, L))[1].toJSNumber(), n2 && (a2 = L - 1 - a2), i2 = s2[0], u2 = l2[0], f2.push(r2(p2, a2));
          for (var v2 = 0 !== r2(o2 ? 1 : 0, n2 ? 1 : 0) ? bigInt2(-1) : bigInt2(0), h2 = f2.length - 1; h2 >= 0; h2 -= 1) v2 = v2.multiply(L).add(bigInt2(f2[h2]));
          return v2;
        }
        p.prototype.shiftLeft = function(t2) {
          var e2 = K(t2).toJSNumber();
          if (!U(e2)) throw new Error(String(e2) + " is too large for shifting.");
          if (e2 < 0) return this.shiftRight(-e2);
          var r2 = this;
          if (r2.isZero()) return r2;
          for (; e2 >= J; ) r2 = r2.multiply(L), e2 -= J - 1;
          return r2.multiply(x[e2]);
        }, s.prototype.shiftLeft = a.prototype.shiftLeft = p.prototype.shiftLeft, p.prototype.shiftRight = function(t2) {
          var e2, r2 = K(t2).toJSNumber();
          if (!U(r2)) throw new Error(String(r2) + " is too large for shifting.");
          if (r2 < 0) return this.shiftLeft(-r2);
          for (var o2 = this; r2 >= J; ) {
            if (o2.isZero() || o2.isNegative() && o2.isUnit()) return o2;
            o2 = (e2 = B(o2, L))[1].isNegative() ? e2[0].prev() : e2[0], r2 -= J - 1;
          }
          return (e2 = B(o2, x[r2]))[1].isNegative() ? e2[0].prev() : e2[0];
        }, s.prototype.shiftRight = a.prototype.shiftRight = p.prototype.shiftRight, p.prototype.not = function() {
          return this.negate().prev();
        }, s.prototype.not = a.prototype.not = p.prototype.not, p.prototype.and = function(t2) {
          return T(this, t2, function(t3, e2) {
            return t3 & e2;
          });
        }, s.prototype.and = a.prototype.and = p.prototype.and, p.prototype.or = function(t2) {
          return T(this, t2, function(t3, e2) {
            return t3 | e2;
          });
        }, s.prototype.or = a.prototype.or = p.prototype.or, p.prototype.xor = function(t2) {
          return T(this, t2, function(t3, e2) {
            return t3 ^ e2;
          });
        }, s.prototype.xor = a.prototype.xor = p.prototype.xor;
        var j = 1 << 30;
        function C(t2) {
          var r2 = t2.value, o2 = "number" == typeof r2 ? r2 | j : "bigint" == typeof r2 ? r2 | BigInt(j) : r2[0] + r2[1] * e | 1073758208;
          return o2 & -o2;
        }
        function D(t2, e2) {
          if (e2.compareTo(t2) <= 0) {
            var r2 = D(t2, e2.square(e2)), o2 = r2.p, n2 = r2.e, i2 = o2.multiply(e2);
            return i2.compareTo(t2) <= 0 ? { p: i2, e: 2 * n2 + 1 } : { p: o2, e: 2 * n2 };
          }
          return { p: bigInt2(1), e: 0 };
        }
        function z(t2, e2) {
          return t2 = K(t2), e2 = K(e2), t2.greater(e2) ? t2 : e2;
        }
        function R(t2, e2) {
          return t2 = K(t2), e2 = K(e2), t2.lesser(e2) ? t2 : e2;
        }
        function k(t2, e2) {
          if (t2 = K(t2).abs(), e2 = K(e2).abs(), t2.equals(e2)) return t2;
          if (t2.isZero()) return e2;
          if (e2.isZero()) return t2;
          for (var r2, o2, n2 = u[1]; t2.isEven() && e2.isEven(); ) r2 = R(C(t2), C(e2)), t2 = t2.divide(r2), e2 = e2.divide(r2), n2 = n2.multiply(r2);
          for (; t2.isEven(); ) t2 = t2.divide(C(t2));
          do {
            for (; e2.isEven(); ) e2 = e2.divide(C(e2));
            t2.greater(e2) && (o2 = e2, e2 = t2, t2 = o2), e2 = e2.subtract(t2);
          } while (!e2.isZero());
          return n2.isUnit() ? t2 : t2.multiply(n2);
        }
        p.prototype.bitLength = function() {
          var t2 = this;
          return t2.compareTo(bigInt2(0)) < 0 && (t2 = t2.negate().subtract(bigInt2(1))), 0 === t2.compareTo(bigInt2(0)) ? bigInt2(0) : bigInt2(D(t2, bigInt2(2)).e).add(bigInt2(1));
        }, s.prototype.bitLength = a.prototype.bitLength = p.prototype.bitLength;
        var _ = function(t2, e2, r2, o2) {
          r2 = r2 || n, t2 = String(t2), o2 || (t2 = t2.toLowerCase(), r2 = r2.toLowerCase());
          var i2, u2 = t2.length, p2 = Math.abs(e2), a2 = {};
          for (i2 = 0; i2 < r2.length; i2++) a2[r2[i2]] = i2;
          for (i2 = 0; i2 < u2; i2++) {
            if ("-" !== (f2 = t2[i2]) && (f2 in a2 && a2[f2] >= p2)) {
              if ("1" === f2 && 1 === p2) continue;
              throw new Error(f2 + " is not a valid digit in base " + e2 + ".");
            }
          }
          e2 = K(e2);
          var s2 = [], l2 = "-" === t2[0];
          for (i2 = l2 ? 1 : 0; i2 < t2.length; i2++) {
            var f2;
            if ((f2 = t2[i2]) in a2) s2.push(K(a2[f2]));
            else {
              if ("<" !== f2) throw new Error(f2 + " is not a valid character");
              var v2 = i2;
              do {
                i2++;
              } while (">" !== t2[i2] && i2 < t2.length);
              s2.push(K(t2.slice(v2 + 1, i2)));
            }
          }
          return $(s2, e2, l2);
        };
        function $(t2, e2, r2) {
          var o2, n2 = u[0], i2 = u[1];
          for (o2 = t2.length - 1; o2 >= 0; o2--) n2 = n2.add(t2[o2].times(i2)), i2 = i2.times(e2);
          return r2 ? n2.negate() : n2;
        }
        function F(t2, e2) {
          if ((e2 = bigInt2(e2)).isZero()) {
            if (t2.isZero()) return { value: [0], isNegative: false };
            throw new Error("Cannot convert nonzero numbers to base 0.");
          }
          if (e2.equals(-1)) {
            if (t2.isZero()) return { value: [0], isNegative: false };
            if (t2.isNegative()) return { value: [].concat.apply([], Array.apply(null, Array(-t2.toJSNumber())).map(Array.prototype.valueOf, [1, 0])), isNegative: false };
            var r2 = Array.apply(null, Array(t2.toJSNumber() - 1)).map(Array.prototype.valueOf, [0, 1]);
            return r2.unshift([1]), { value: [].concat.apply([], r2), isNegative: false };
          }
          var o2 = false;
          if (t2.isNegative() && e2.isPositive() && (o2 = true, t2 = t2.abs()), e2.isUnit()) return t2.isZero() ? { value: [0], isNegative: false } : { value: Array.apply(null, Array(t2.toJSNumber())).map(Number.prototype.valueOf, 1), isNegative: o2 };
          for (var n2, i2 = [], u2 = t2; u2.isNegative() || u2.compareAbs(e2) >= 0; ) {
            n2 = u2.divmod(e2), u2 = n2.quotient;
            var p2 = n2.remainder;
            p2.isNegative() && (p2 = e2.minus(p2).abs(), u2 = u2.next()), i2.push(p2.toJSNumber());
          }
          return i2.push(u2.toJSNumber()), { value: i2.reverse(), isNegative: o2 };
        }
        function G(t2, e2, r2) {
          var o2 = F(t2, e2);
          return (o2.isNegative ? "-" : "") + o2.value.map(function(t3) {
            return function(t4, e3) {
              return t4 < (e3 = e3 || n).length ? e3[t4] : "<" + t4 + ">";
            }(t3, r2);
          }).join("");
        }
        function H(t2) {
          if (l(+t2)) {
            var e2 = +t2;
            if (e2 === g(e2)) return i ? new s(BigInt(e2)) : new a(e2);
            throw new Error("Invalid integer: " + t2);
          }
          var r2 = "-" === t2[0];
          r2 && (t2 = t2.slice(1));
          var o2 = t2.split(/e/i);
          if (o2.length > 2) throw new Error("Invalid integer: " + o2.join("e"));
          if (2 === o2.length) {
            var n2 = o2[1];
            if ("+" === n2[0] && (n2 = n2.slice(1)), (n2 = +n2) !== g(n2) || !l(n2)) throw new Error("Invalid integer: " + n2 + " is not a valid exponent.");
            var u2 = o2[0], f2 = u2.indexOf(".");
            if (f2 >= 0 && (n2 -= u2.length - f2 - 1, u2 = u2.slice(0, f2) + u2.slice(f2 + 1)), n2 < 0) throw new Error("Cannot include negative exponent part for integers");
            t2 = u2 += new Array(n2 + 1).join("0");
          }
          if (!/^([0-9][0-9]*)$/.test(t2)) throw new Error("Invalid integer: " + t2);
          if (i) return new s(BigInt(r2 ? "-" + t2 : t2));
          for (var v2 = [], y2 = t2.length, c2 = y2 - 7; y2 > 0; ) v2.push(+t2.slice(c2, y2)), (c2 -= 7) < 0 && (c2 = 0), y2 -= 7;
          return h(v2), new p(v2, r2);
        }
        function K(t2) {
          return "number" == typeof t2 ? function(t3) {
            if (i) return new s(BigInt(t3));
            if (l(t3)) {
              if (t3 !== g(t3)) throw new Error(t3 + " is not an integer.");
              return new a(t3);
            }
            return H(t3.toString());
          }(t2) : "string" == typeof t2 ? H(t2) : "bigint" == typeof t2 ? new s(t2) : t2;
        }
        p.prototype.toArray = function(t2) {
          return F(this, t2);
        }, a.prototype.toArray = function(t2) {
          return F(this, t2);
        }, s.prototype.toArray = function(t2) {
          return F(this, t2);
        }, p.prototype.toString = function(e2, r2) {
          if (e2 === t && (e2 = 10), 10 !== e2) return G(this, e2, r2);
          for (var o2, n2 = this.value, i2 = n2.length, u2 = String(n2[--i2]); --i2 >= 0; ) o2 = String(n2[i2]), u2 += "0000000".slice(o2.length) + o2;
          return (this.sign ? "-" : "") + u2;
        }, a.prototype.toString = function(e2, r2) {
          return e2 === t && (e2 = 10), 10 != e2 ? G(this, e2, r2) : String(this.value);
        }, s.prototype.toString = a.prototype.toString, s.prototype.toJSON = p.prototype.toJSON = a.prototype.toJSON = function() {
          return this.toString();
        }, p.prototype.valueOf = function() {
          return parseInt(this.toString(), 10);
        }, p.prototype.toJSNumber = p.prototype.valueOf, a.prototype.valueOf = function() {
          return this.value;
        }, a.prototype.toJSNumber = a.prototype.valueOf, s.prototype.valueOf = s.prototype.toJSNumber = function() {
          return parseInt(this.toString(), 10);
        };
        for (var Q = 0; Q < 1e3; Q++) u[Q] = K(Q), Q > 0 && (u[-Q] = K(-Q));
        return u.one = u[1], u.zero = u[0], u.minusOne = u[-1], u.max = z, u.min = R, u.gcd = k, u.lcm = function(t2, e2) {
          return t2 = K(t2).abs(), e2 = K(e2).abs(), t2.divide(k(t2, e2)).multiply(e2);
        }, u.isInstance = function(t2) {
          return t2 instanceof p || t2 instanceof a || t2 instanceof s;
        }, u.randBetween = function(t2, r2, o2) {
          t2 = K(t2), r2 = K(r2);
          var n2 = o2 || Math.random, i2 = R(t2, r2), p2 = z(t2, r2).subtract(i2).add(1);
          if (p2.isSmall) return i2.add(Math.floor(n2() * p2));
          for (var a2 = F(p2, e).value, s2 = [], l2 = true, f2 = 0; f2 < a2.length; f2++) {
            var v2 = l2 ? a2[f2] + (f2 + 1 < a2.length ? a2[f2 + 1] / e : 0) : e, h2 = g(n2() * v2);
            s2.push(h2), h2 < a2[f2] && (l2 = false);
          }
          return i2.add(u.fromArray(s2, e, false));
        }, u.fromArray = function(t2, e2, r2) {
          return $(t2.map(K), K(e2 || 10), r2);
        }, u;
      }();
      "undefined" != typeof module && module.hasOwnProperty("exports") && (module.exports = bigInt2), "function" == typeof define && define.amd && define(function() {
        return bigInt2;
      });
    }
  });

  // runtime/prelude.ts
  var import_bigInt = __toESM(require_bigInt());

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/names.js
  function protoCamelCase(snakeCase) {
    let capNext = false;
    const b = [];
    for (let i = 0; i < snakeCase.length; i++) {
      let c = snakeCase.charAt(i);
      switch (c) {
        case "_":
          capNext = true;
          break;
        case "0":
        case "1":
        case "2":
        case "3":
        case "4":
        case "5":
        case "6":
        case "7":
        case "8":
        case "9":
          b.push(c);
          capNext = false;
          break;
        default:
          if (capNext) {
            capNext = false;
            c = c.toUpperCase();
          }
          b.push(c);
          break;
      }
    }
    return b.join("");
  }
  var reservedObjectProperties = /* @__PURE__ */ new Set([
    // names reserved by JavaScript
    "constructor",
    "toString",
    "toJSON",
    "valueOf"
  ]);
  function safeObjectProperty(name) {
    return reservedObjectProperties.has(name) ? name + "$" : name;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wire/varint.js
  function varint64read() {
    let lowBits = 0;
    let highBits = 0;
    for (let shift = 0; shift < 28; shift += 7) {
      let b = this.buf[this.pos++];
      lowBits |= (b & 127) << shift;
      if ((b & 128) == 0) {
        this.assertBounds();
        return [lowBits, highBits];
      }
    }
    let middleByte = this.buf[this.pos++];
    lowBits |= (middleByte & 15) << 28;
    highBits = (middleByte & 112) >> 4;
    if ((middleByte & 128) == 0) {
      this.assertBounds();
      return [lowBits, highBits];
    }
    for (let shift = 3; shift <= 31; shift += 7) {
      let b = this.buf[this.pos++];
      highBits |= (b & 127) << shift;
      if ((b & 128) == 0) {
        this.assertBounds();
        return [lowBits, highBits];
      }
    }
    throw new Error("invalid varint");
  }
  var TWO_PWR_32_DBL = 4294967296;
  function int64FromString(dec) {
    const minus = dec[0] === "-";
    if (minus) {
      dec = dec.slice(1);
    }
    const base = 1e6;
    let lowBits = 0;
    let highBits = 0;
    function add1e6digit(begin, end) {
      const digit1e6 = Number(dec.slice(begin, end));
      highBits *= base;
      lowBits = lowBits * base + digit1e6;
      if (lowBits >= TWO_PWR_32_DBL) {
        highBits = highBits + (lowBits / TWO_PWR_32_DBL | 0);
        lowBits = lowBits % TWO_PWR_32_DBL;
      }
    }
    add1e6digit(-24, -18);
    add1e6digit(-18, -12);
    add1e6digit(-12, -6);
    add1e6digit(-6);
    return minus ? negate(lowBits, highBits) : newBits(lowBits, highBits);
  }
  function int64ToString(lo, hi) {
    let bits = newBits(lo, hi);
    const negative = bits.hi & 2147483648;
    if (negative) {
      bits = negate(bits.lo, bits.hi);
    }
    const result = uInt64ToString(bits.lo, bits.hi);
    return negative ? "-" + result : result;
  }
  function uInt64ToString(lo, hi) {
    ({ lo, hi } = toUnsigned(lo, hi));
    if (hi <= 2097151) {
      return String(TWO_PWR_32_DBL * hi + lo);
    }
    const low = lo & 16777215;
    const mid = (lo >>> 24 | hi << 8) & 16777215;
    const high = hi >> 16 & 65535;
    let digitA = low + mid * 6777216 + high * 6710656;
    let digitB = mid + high * 8147497;
    let digitC = high * 2;
    const base = 1e7;
    if (digitA >= base) {
      digitB += Math.floor(digitA / base);
      digitA %= base;
    }
    if (digitB >= base) {
      digitC += Math.floor(digitB / base);
      digitB %= base;
    }
    return digitC.toString() + decimalFrom1e7WithLeadingZeros(digitB) + decimalFrom1e7WithLeadingZeros(digitA);
  }
  function toUnsigned(lo, hi) {
    return { lo: lo >>> 0, hi: hi >>> 0 };
  }
  function newBits(lo, hi) {
    return { lo: lo | 0, hi: hi | 0 };
  }
  function negate(lowBits, highBits) {
    highBits = ~highBits;
    if (lowBits) {
      lowBits = ~lowBits + 1;
    } else {
      highBits += 1;
    }
    return newBits(lowBits, highBits);
  }
  var decimalFrom1e7WithLeadingZeros = (digit1e7) => {
    const partial = String(digit1e7);
    return "0000000".slice(partial.length) + partial;
  };
  function varint32read() {
    let b = this.buf[this.pos++];
    let result = b & 127;
    if ((b & 128) == 0) {
      this.assertBounds();
      return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 127) << 7;
    if ((b & 128) == 0) {
      this.assertBounds();
      return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 127) << 14;
    if ((b & 128) == 0) {
      this.assertBounds();
      return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 127) << 21;
    if ((b & 128) == 0) {
      this.assertBounds();
      return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 15) << 28;
    for (let readBytes = 5; (b & 128) !== 0 && readBytes < 10; readBytes++)
      b = this.buf[this.pos++];
    if ((b & 128) != 0)
      throw new Error("invalid varint");
    this.assertBounds();
    return result >>> 0;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/proto-int64.js
  var protoInt64 = /* @__PURE__ */ makeInt64Support();
  function makeInt64Support() {
    const dv = new DataView(new ArrayBuffer(8));
    const ok = typeof BigInt === "function" && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function" && (typeof process != "object" || typeof process.env != "object" || process.env.BUF_BIGINT_DISABLE !== "1");
    if (ok) {
      const MIN = BigInt("-9223372036854775808");
      const MAX = BigInt("9223372036854775807");
      const UMIN = BigInt("0");
      const UMAX = BigInt("18446744073709551615");
      return {
        zero: BigInt(0),
        supported: true,
        parse(value) {
          const bi = typeof value == "bigint" ? value : BigInt(value);
          if (bi > MAX || bi < MIN) {
            throw new Error(`invalid int64: ${value}`);
          }
          return bi;
        },
        uParse(value) {
          const bi = typeof value == "bigint" ? value : BigInt(value);
          if (bi > UMAX || bi < UMIN) {
            throw new Error(`invalid uint64: ${value}`);
          }
          return bi;
        },
        enc(value) {
          dv.setBigInt64(0, this.parse(value), true);
          return {
            lo: dv.getInt32(0, true),
            hi: dv.getInt32(4, true)
          };
        },
        uEnc(value) {
          dv.setBigInt64(0, this.uParse(value), true);
          return {
            lo: dv.getInt32(0, true),
            hi: dv.getInt32(4, true)
          };
        },
        dec(lo, hi) {
          dv.setInt32(0, lo, true);
          dv.setInt32(4, hi, true);
          return dv.getBigInt64(0, true);
        },
        uDec(lo, hi) {
          dv.setInt32(0, lo, true);
          dv.setInt32(4, hi, true);
          return dv.getBigUint64(0, true);
        }
      };
    }
    return {
      zero: "0",
      supported: false,
      parse(value) {
        if (typeof value != "string") {
          value = value.toString();
        }
        assertInt64String(value);
        return value;
      },
      uParse(value) {
        if (typeof value != "string") {
          value = value.toString();
        }
        assertUInt64String(value);
        return value;
      },
      enc(value) {
        if (typeof value != "string") {
          value = value.toString();
        }
        assertInt64String(value);
        return int64FromString(value);
      },
      uEnc(value) {
        if (typeof value != "string") {
          value = value.toString();
        }
        assertUInt64String(value);
        return int64FromString(value);
      },
      dec(lo, hi) {
        return int64ToString(lo, hi);
      },
      uDec(lo, hi) {
        return uInt64ToString(lo, hi);
      }
    };
  }
  function assertInt64String(value) {
    if (!/^-?[0-9]+$/.test(value)) {
      throw new Error("invalid int64: " + value);
    }
  }
  function assertUInt64String(value) {
    if (!/^[0-9]+$/.test(value)) {
      throw new Error("invalid uint64: " + value);
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/descriptors.js
  var ScalarType;
  (function(ScalarType2) {
    ScalarType2[ScalarType2["DOUBLE"] = 1] = "DOUBLE";
    ScalarType2[ScalarType2["FLOAT"] = 2] = "FLOAT";
    ScalarType2[ScalarType2["INT64"] = 3] = "INT64";
    ScalarType2[ScalarType2["UINT64"] = 4] = "UINT64";
    ScalarType2[ScalarType2["INT32"] = 5] = "INT32";
    ScalarType2[ScalarType2["FIXED64"] = 6] = "FIXED64";
    ScalarType2[ScalarType2["FIXED32"] = 7] = "FIXED32";
    ScalarType2[ScalarType2["BOOL"] = 8] = "BOOL";
    ScalarType2[ScalarType2["STRING"] = 9] = "STRING";
    ScalarType2[ScalarType2["BYTES"] = 12] = "BYTES";
    ScalarType2[ScalarType2["UINT32"] = 13] = "UINT32";
    ScalarType2[ScalarType2["SFIXED32"] = 15] = "SFIXED32";
    ScalarType2[ScalarType2["SFIXED64"] = 16] = "SFIXED64";
    ScalarType2[ScalarType2["SINT32"] = 17] = "SINT32";
    ScalarType2[ScalarType2["SINT64"] = 18] = "SINT64";
  })(ScalarType || (ScalarType = {}));

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/scalar.js
  function scalarZeroValue(type, longAsString) {
    switch (type) {
      case ScalarType.STRING:
        return "";
      case ScalarType.BOOL:
        return false;
      case ScalarType.DOUBLE:
      case ScalarType.FLOAT:
        return 0;
      case ScalarType.INT64:
      case ScalarType.UINT64:
      case ScalarType.SFIXED64:
      case ScalarType.FIXED64:
      case ScalarType.SINT64:
        return longAsString ? "0" : protoInt64.zero;
      case ScalarType.BYTES:
        return new Uint8Array(0);
      default:
        return 0;
    }
  }
  function isScalarZeroValue(type, value) {
    switch (type) {
      case ScalarType.BOOL:
        return value === false;
      case ScalarType.STRING:
        return value === "";
      case ScalarType.BYTES:
        return value instanceof Uint8Array && !value.byteLength;
      default:
        return value == 0;
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/unsafe.js
  var IMPLICIT = 2;
  var unsafeLocal = Symbol.for("reflect unsafe local");
  function unsafeOneofCase(target, oneof) {
    const c = target[oneof.localName].case;
    if (c === void 0) {
      return c;
    }
    return oneof.fields.find((f) => f.localName === c);
  }
  function unsafeIsSet(target, field) {
    const name = field.localName;
    if (field.oneof) {
      return target[field.oneof.localName].case === name;
    }
    if (field.presence != IMPLICIT) {
      return target[name] !== void 0 && Object.prototype.hasOwnProperty.call(target, name);
    }
    switch (field.fieldKind) {
      case "list":
        return target[name].length > 0;
      case "map":
        return Object.keys(target[name]).length > 0;
      case "scalar":
        return !isScalarZeroValue(field.scalar, target[name]);
      case "enum":
        return target[name] !== field.enum.values[0].number;
    }
    throw new Error("message field with implicit presence");
  }
  function unsafeIsSetExplicit(target, localName) {
    return Object.prototype.hasOwnProperty.call(target, localName) && target[localName] !== void 0;
  }
  function unsafeGet(target, field) {
    if (field.oneof) {
      const oneof = target[field.oneof.localName];
      if (oneof.case === field.localName) {
        return oneof.value;
      }
      return void 0;
    }
    return target[field.localName];
  }
  function unsafeSet(target, field, value) {
    if (field.oneof) {
      target[field.oneof.localName] = {
        case: field.localName,
        value
      };
    } else {
      target[field.localName] = value;
    }
  }
  function unsafeClear(target, field) {
    const name = field.localName;
    if (field.oneof) {
      const oneofLocalName = field.oneof.localName;
      if (target[oneofLocalName].case === name) {
        target[oneofLocalName] = { case: void 0 };
      }
    } else if (field.presence != IMPLICIT) {
      delete target[name];
    } else {
      switch (field.fieldKind) {
        case "map":
          target[name] = {};
          break;
        case "list":
          target[name] = [];
          break;
        case "enum":
          target[name] = field.enum.values[0].number;
          break;
        case "scalar":
          target[name] = scalarZeroValue(field.scalar, field.longAsString);
          break;
      }
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/codegenv1/restore-json-names.js
  function restoreJsonNames(message) {
    for (const f of message.field) {
      if (!unsafeIsSetExplicit(f, "jsonName")) {
        f.jsonName = protoCamelCase(f.name);
      }
    }
    message.nestedType.forEach(restoreJsonNames);
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wire/text-format.js
  function parseTextFormatEnumValue(descEnum, value) {
    const enumValue = descEnum.values.find((v) => v.name === value);
    if (!enumValue) {
      throw new Error(`cannot parse ${descEnum} default value: ${value}`);
    }
    return enumValue.number;
  }
  function parseTextFormatScalarValue(type, value) {
    switch (type) {
      case ScalarType.STRING:
        return value;
      case ScalarType.BYTES: {
        const u = unescapeBytesDefaultValue(value);
        if (u === false) {
          throw new Error(`cannot parse ${ScalarType[type]} default value: ${value}`);
        }
        return u;
      }
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        return protoInt64.parse(value);
      case ScalarType.UINT64:
      case ScalarType.FIXED64:
        return protoInt64.uParse(value);
      case ScalarType.DOUBLE:
      case ScalarType.FLOAT:
        switch (value) {
          case "inf":
            return Number.POSITIVE_INFINITY;
          case "-inf":
            return Number.NEGATIVE_INFINITY;
          case "nan":
            return Number.NaN;
          default:
            return parseFloat(value);
        }
      case ScalarType.BOOL:
        return value === "true";
      case ScalarType.INT32:
      case ScalarType.UINT32:
      case ScalarType.SINT32:
      case ScalarType.FIXED32:
      case ScalarType.SFIXED32:
        return parseInt(value, 10);
    }
  }
  function unescapeBytesDefaultValue(str) {
    const b = [];
    const input = {
      tail: str,
      c: "",
      next() {
        if (this.tail.length == 0) {
          return false;
        }
        this.c = this.tail[0];
        this.tail = this.tail.substring(1);
        return true;
      },
      take(n) {
        if (this.tail.length >= n) {
          const r = this.tail.substring(0, n);
          this.tail = this.tail.substring(n);
          return r;
        }
        return false;
      }
    };
    while (input.next()) {
      switch (input.c) {
        case "\\":
          if (input.next()) {
            switch (input.c) {
              case "\\":
                b.push(input.c.charCodeAt(0));
                break;
              case "b":
                b.push(8);
                break;
              case "f":
                b.push(12);
                break;
              case "n":
                b.push(10);
                break;
              case "r":
                b.push(13);
                break;
              case "t":
                b.push(9);
                break;
              case "v":
                b.push(11);
                break;
              case "0":
              case "1":
              case "2":
              case "3":
              case "4":
              case "5":
              case "6":
              case "7": {
                const s = input.c;
                const t = input.take(2);
                if (t === false) {
                  return false;
                }
                const n = parseInt(s + t, 8);
                if (Number.isNaN(n)) {
                  return false;
                }
                b.push(n);
                break;
              }
              case "x": {
                const s = input.c;
                const t = input.take(2);
                if (t === false) {
                  return false;
                }
                const n = parseInt(s + t, 16);
                if (Number.isNaN(n)) {
                  return false;
                }
                b.push(n);
                break;
              }
              case "u": {
                const s = input.c;
                const t = input.take(4);
                if (t === false) {
                  return false;
                }
                const n = parseInt(s + t, 16);
                if (Number.isNaN(n)) {
                  return false;
                }
                const chunk = new Uint8Array(4);
                const view = new DataView(chunk.buffer);
                view.setInt32(0, n, true);
                b.push(chunk[0], chunk[1], chunk[2], chunk[3]);
                break;
              }
              case "U": {
                const s = input.c;
                const t = input.take(8);
                if (t === false) {
                  return false;
                }
                const tc = protoInt64.uEnc(s + t);
                const chunk = new Uint8Array(8);
                const view = new DataView(chunk.buffer);
                view.setInt32(0, tc.lo, true);
                view.setInt32(4, tc.hi, true);
                b.push(chunk[0], chunk[1], chunk[2], chunk[3], chunk[4], chunk[5], chunk[6], chunk[7]);
                break;
              }
            }
          }
          break;
        default:
          b.push(input.c.charCodeAt(0));
      }
    }
    return new Uint8Array(b);
  }

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/nested-types.js
  function* nestedTypes(desc) {
    switch (desc.kind) {
      case "file":
        for (const message of desc.messages) {
          yield message;
          yield* __yieldStar(nestedTypes(message));
        }
        yield* __yieldStar(desc.enums);
        yield* __yieldStar(desc.services);
        yield* __yieldStar(desc.extensions);
        break;
      case "message":
        for (const message of desc.nestedMessages) {
          yield message;
          yield* __yieldStar(nestedTypes(message));
        }
        yield* __yieldStar(desc.nestedEnums);
        yield* __yieldStar(desc.nestedExtensions);
        break;
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/registry.js
  function createFileRegistry(...args) {
    const registry = createBaseRegistry();
    if (!args.length) {
      return registry;
    }
    if ("$typeName" in args[0] && args[0].$typeName == "google.protobuf.FileDescriptorSet") {
      for (const file of args[0].file) {
        addFile(file, registry);
      }
      return registry;
    }
    if ("$typeName" in args[0]) {
      let recurseDeps = function(file) {
        const deps = [];
        for (const protoFileName of file.dependency) {
          if (registry.getFile(protoFileName) != void 0) {
            continue;
          }
          if (seen.has(protoFileName)) {
            continue;
          }
          const dep = resolve(protoFileName);
          if (!dep) {
            throw new Error(`Unable to resolve ${protoFileName}, imported by ${file.name}`);
          }
          if ("kind" in dep) {
            registry.addFile(dep, false, true);
          } else {
            seen.add(dep.name);
            deps.push(dep);
          }
        }
        return deps.concat(...deps.map(recurseDeps));
      };
      const input = args[0];
      const resolve = args[1];
      const seen = /* @__PURE__ */ new Set();
      for (const file of [input, ...recurseDeps(input)].reverse()) {
        addFile(file, registry);
      }
    } else {
      for (const fileReg of args) {
        for (const file of fileReg.files) {
          registry.addFile(file);
        }
      }
    }
    return registry;
  }
  function createBaseRegistry() {
    const types = /* @__PURE__ */ new Map();
    const extendees = /* @__PURE__ */ new Map();
    const files = /* @__PURE__ */ new Map();
    return {
      kind: "registry",
      types,
      extendees,
      [Symbol.iterator]() {
        return types.values();
      },
      get files() {
        return files.values();
      },
      addFile(file, skipTypes, withDeps) {
        files.set(file.proto.name, file);
        if (!skipTypes) {
          for (const type of nestedTypes(file)) {
            this.add(type);
          }
        }
        if (withDeps) {
          for (const f of file.dependencies) {
            this.addFile(f, skipTypes, withDeps);
          }
        }
      },
      add(desc) {
        if (desc.kind == "extension") {
          let numberToExt = extendees.get(desc.extendee.typeName);
          if (!numberToExt) {
            extendees.set(
              desc.extendee.typeName,
              // biome-ignore lint/suspicious/noAssignInExpressions: no
              numberToExt = /* @__PURE__ */ new Map()
            );
          }
          numberToExt.set(desc.number, desc);
        }
        types.set(desc.typeName, desc);
      },
      get(typeName) {
        return types.get(typeName);
      },
      getFile(fileName) {
        return files.get(fileName);
      },
      getMessage(typeName) {
        const t = types.get(typeName);
        return (t === null || t === void 0 ? void 0 : t.kind) == "message" ? t : void 0;
      },
      getEnum(typeName) {
        const t = types.get(typeName);
        return (t === null || t === void 0 ? void 0 : t.kind) == "enum" ? t : void 0;
      },
      getExtension(typeName) {
        const t = types.get(typeName);
        return (t === null || t === void 0 ? void 0 : t.kind) == "extension" ? t : void 0;
      },
      getExtensionFor(extendee, no) {
        var _a;
        return (_a = extendees.get(extendee.typeName)) === null || _a === void 0 ? void 0 : _a.get(no);
      },
      getService(typeName) {
        const t = types.get(typeName);
        return (t === null || t === void 0 ? void 0 : t.kind) == "service" ? t : void 0;
      }
    };
  }
  var EDITION_PROTO2 = 998;
  var EDITION_PROTO3 = 999;
  var TYPE_STRING = 9;
  var TYPE_GROUP = 10;
  var TYPE_MESSAGE = 11;
  var TYPE_BYTES = 12;
  var TYPE_ENUM = 14;
  var LABEL_REPEATED = 3;
  var LABEL_REQUIRED = 2;
  var JS_STRING = 1;
  var IDEMPOTENCY_UNKNOWN = 0;
  var EXPLICIT = 1;
  var IMPLICIT2 = 2;
  var LEGACY_REQUIRED = 3;
  var PACKED = 1;
  var DELIMITED = 2;
  var OPEN = 1;
  var featureDefaults = {
    // EDITION_PROTO2
    998: {
      fieldPresence: 1,
      // EXPLICIT,
      enumType: 2,
      // CLOSED,
      repeatedFieldEncoding: 2,
      // EXPANDED,
      utf8Validation: 3,
      // NONE,
      messageEncoding: 1,
      // LENGTH_PREFIXED,
      jsonFormat: 2,
      // LEGACY_BEST_EFFORT,
      enforceNamingStyle: 2
      // STYLE_LEGACY,
    },
    // EDITION_PROTO3
    999: {
      fieldPresence: 2,
      // IMPLICIT,
      enumType: 1,
      // OPEN,
      repeatedFieldEncoding: 1,
      // PACKED,
      utf8Validation: 2,
      // VERIFY,
      messageEncoding: 1,
      // LENGTH_PREFIXED,
      jsonFormat: 1,
      // ALLOW,
      enforceNamingStyle: 2
      // STYLE_LEGACY,
    },
    // EDITION_2023
    1e3: {
      fieldPresence: 1,
      // EXPLICIT,
      enumType: 1,
      // OPEN,
      repeatedFieldEncoding: 1,
      // PACKED,
      utf8Validation: 2,
      // VERIFY,
      messageEncoding: 1,
      // LENGTH_PREFIXED,
      jsonFormat: 1,
      // ALLOW,
      enforceNamingStyle: 2
      // STYLE_LEGACY,
    }
  };
  function addFile(proto, reg) {
    var _a, _b;
    const file = {
      kind: "file",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      edition: getFileEdition(proto),
      name: proto.name.replace(/\.proto$/, ""),
      dependencies: findFileDependencies(proto, reg),
      enums: [],
      messages: [],
      extensions: [],
      services: [],
      toString() {
        return `file ${proto.name}`;
      }
    };
    const mapEntriesStore = /* @__PURE__ */ new Map();
    const mapEntries = {
      get(typeName) {
        return mapEntriesStore.get(typeName);
      },
      add(desc) {
        var _a2;
        assert(((_a2 = desc.proto.options) === null || _a2 === void 0 ? void 0 : _a2.mapEntry) === true);
        mapEntriesStore.set(desc.typeName, desc);
      }
    };
    for (const enumProto of proto.enumType) {
      addEnum(enumProto, file, void 0, reg);
    }
    for (const messageProto of proto.messageType) {
      addMessage(messageProto, file, void 0, reg, mapEntries);
    }
    for (const serviceProto of proto.service) {
      addService(serviceProto, file, reg);
    }
    addExtensions(file, reg);
    for (const mapEntry of mapEntriesStore.values()) {
      addFields(mapEntry, reg, mapEntries);
    }
    for (const message of file.messages) {
      addFields(message, reg, mapEntries);
      addExtensions(message, reg);
    }
    reg.addFile(file, true);
  }
  function addExtensions(desc, reg) {
    switch (desc.kind) {
      case "file":
        for (const proto of desc.proto.extension) {
          const ext = newField(proto, desc, reg);
          desc.extensions.push(ext);
          reg.add(ext);
        }
        break;
      case "message":
        for (const proto of desc.proto.extension) {
          const ext = newField(proto, desc, reg);
          desc.nestedExtensions.push(ext);
          reg.add(ext);
        }
        for (const message of desc.nestedMessages) {
          addExtensions(message, reg);
        }
        break;
    }
  }
  function addFields(message, reg, mapEntries) {
    const allOneofs = message.proto.oneofDecl.map((proto) => newOneof(proto, message));
    const oneofsSeen = /* @__PURE__ */ new Set();
    for (const proto of message.proto.field) {
      const oneof = findOneof(proto, allOneofs);
      const field = newField(proto, message, reg, oneof, mapEntries);
      message.fields.push(field);
      message.field[field.localName] = field;
      if (oneof === void 0) {
        message.members.push(field);
      } else {
        oneof.fields.push(field);
        if (!oneofsSeen.has(oneof)) {
          oneofsSeen.add(oneof);
          message.members.push(oneof);
        }
      }
    }
    for (const oneof of allOneofs.filter((o) => oneofsSeen.has(o))) {
      message.oneofs.push(oneof);
    }
    for (const child of message.nestedMessages) {
      addFields(child, reg, mapEntries);
    }
  }
  function addEnum(proto, file, parent, reg) {
    var _a, _b, _c, _d, _e;
    const sharedPrefix = findEnumSharedPrefix(proto.name, proto.value);
    const desc = {
      kind: "enum",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      file,
      parent,
      open: true,
      name: proto.name,
      typeName: makeTypeName(proto, parent, file),
      value: {},
      values: [],
      sharedPrefix,
      toString() {
        return `enum ${this.typeName}`;
      }
    };
    desc.open = isEnumOpen(desc);
    reg.add(desc);
    for (const p of proto.value) {
      const name = p.name;
      desc.values.push(
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        desc.value[p.number] = {
          kind: "enum_value",
          proto: p,
          deprecated: (_d = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.deprecated) !== null && _d !== void 0 ? _d : false,
          parent: desc,
          name,
          localName: safeObjectProperty(sharedPrefix == void 0 ? name : name.substring(sharedPrefix.length)),
          number: p.number,
          toString() {
            return `enum value ${desc.typeName}.${name}`;
          }
        }
      );
    }
    ((_e = parent === null || parent === void 0 ? void 0 : parent.nestedEnums) !== null && _e !== void 0 ? _e : file.enums).push(desc);
  }
  function addMessage(proto, file, parent, reg, mapEntries) {
    var _a, _b, _c, _d;
    const desc = {
      kind: "message",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      file,
      parent,
      name: proto.name,
      typeName: makeTypeName(proto, parent, file),
      fields: [],
      field: {},
      oneofs: [],
      members: [],
      nestedEnums: [],
      nestedMessages: [],
      nestedExtensions: [],
      toString() {
        return `message ${this.typeName}`;
      }
    };
    if (((_c = proto.options) === null || _c === void 0 ? void 0 : _c.mapEntry) === true) {
      mapEntries.add(desc);
    } else {
      ((_d = parent === null || parent === void 0 ? void 0 : parent.nestedMessages) !== null && _d !== void 0 ? _d : file.messages).push(desc);
      reg.add(desc);
    }
    for (const enumProto of proto.enumType) {
      addEnum(enumProto, file, desc, reg);
    }
    for (const messageProto of proto.nestedType) {
      addMessage(messageProto, file, desc, reg, mapEntries);
    }
  }
  function addService(proto, file, reg) {
    var _a, _b;
    const desc = {
      kind: "service",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      file,
      name: proto.name,
      typeName: makeTypeName(proto, void 0, file),
      methods: [],
      method: {},
      toString() {
        return `service ${this.typeName}`;
      }
    };
    file.services.push(desc);
    reg.add(desc);
    for (const methodProto of proto.method) {
      const method = newMethod(methodProto, desc, reg);
      desc.methods.push(method);
      desc.method[method.localName] = method;
    }
  }
  function newMethod(proto, parent, reg) {
    var _a, _b, _c, _d;
    let methodKind;
    if (proto.clientStreaming && proto.serverStreaming) {
      methodKind = "bidi_streaming";
    } else if (proto.clientStreaming) {
      methodKind = "client_streaming";
    } else if (proto.serverStreaming) {
      methodKind = "server_streaming";
    } else {
      methodKind = "unary";
    }
    const input = reg.getMessage(trimLeadingDot(proto.inputType));
    const output = reg.getMessage(trimLeadingDot(proto.outputType));
    assert(input, `invalid MethodDescriptorProto: input_type ${proto.inputType} not found`);
    assert(output, `invalid MethodDescriptorProto: output_type ${proto.inputType} not found`);
    const name = proto.name;
    return {
      kind: "rpc",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      parent,
      name,
      localName: safeObjectProperty(name.length ? safeObjectProperty(name[0].toLowerCase() + name.substring(1)) : name),
      methodKind,
      input,
      output,
      idempotency: (_d = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.idempotencyLevel) !== null && _d !== void 0 ? _d : IDEMPOTENCY_UNKNOWN,
      toString() {
        return `rpc ${parent.typeName}.${name}`;
      }
    };
  }
  function newOneof(proto, parent) {
    return {
      kind: "oneof",
      proto,
      deprecated: false,
      parent,
      fields: [],
      name: proto.name,
      localName: safeObjectProperty(protoCamelCase(proto.name)),
      toString() {
        return `oneof ${parent.typeName}.${this.name}`;
      }
    };
  }
  function newField(proto, parentOrFile, reg, oneof, mapEntries) {
    var _a, _b, _c;
    const isExtension = mapEntries === void 0;
    const field = {
      kind: "field",
      proto,
      deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
      name: proto.name,
      number: proto.number,
      scalar: void 0,
      message: void 0,
      enum: void 0,
      presence: getFieldPresence(proto, oneof, isExtension, parentOrFile),
      listKind: void 0,
      mapKind: void 0,
      mapKey: void 0,
      delimitedEncoding: void 0,
      packed: void 0,
      longAsString: false,
      getDefaultValue: void 0
    };
    if (isExtension) {
      const file = parentOrFile.kind == "file" ? parentOrFile : parentOrFile.file;
      const parent = parentOrFile.kind == "file" ? void 0 : parentOrFile;
      const typeName = makeTypeName(proto, parent, file);
      field.kind = "extension";
      field.file = file;
      field.parent = parent;
      field.oneof = void 0;
      field.typeName = typeName;
      field.jsonName = `[${typeName}]`;
      field.toString = () => `extension ${typeName}`;
      const extendee = reg.getMessage(trimLeadingDot(proto.extendee));
      assert(extendee, `invalid FieldDescriptorProto: extendee ${proto.extendee} not found`);
      field.extendee = extendee;
    } else {
      const parent = parentOrFile;
      assert(parent.kind == "message");
      field.parent = parent;
      field.oneof = oneof;
      field.localName = oneof ? protoCamelCase(proto.name) : safeObjectProperty(protoCamelCase(proto.name));
      field.jsonName = proto.jsonName;
      field.toString = () => `field ${parent.typeName}.${proto.name}`;
    }
    const label = proto.label;
    const type = proto.type;
    const jstype = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.jstype;
    if (label === LABEL_REPEATED) {
      const mapEntry = type == TYPE_MESSAGE ? mapEntries === null || mapEntries === void 0 ? void 0 : mapEntries.get(trimLeadingDot(proto.typeName)) : void 0;
      if (mapEntry) {
        field.fieldKind = "map";
        const { key, value } = findMapEntryFields(mapEntry);
        field.mapKey = key.scalar;
        field.mapKind = value.fieldKind;
        field.message = value.message;
        field.delimitedEncoding = false;
        field.enum = value.enum;
        field.scalar = value.scalar;
        return field;
      }
      field.fieldKind = "list";
      switch (type) {
        case TYPE_MESSAGE:
        case TYPE_GROUP:
          field.listKind = "message";
          field.message = reg.getMessage(trimLeadingDot(proto.typeName));
          assert(field.message);
          field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
          break;
        case TYPE_ENUM:
          field.listKind = "enum";
          field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
          assert(field.enum);
          break;
        default:
          field.listKind = "scalar";
          field.scalar = type;
          field.longAsString = jstype == JS_STRING;
          break;
      }
      field.packed = isPackedField(proto, parentOrFile);
      return field;
    }
    switch (type) {
      case TYPE_MESSAGE:
      case TYPE_GROUP:
        field.fieldKind = "message";
        field.message = reg.getMessage(trimLeadingDot(proto.typeName));
        assert(field.message, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
        field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
        field.getDefaultValue = () => void 0;
        break;
      case TYPE_ENUM: {
        const enumeration = reg.getEnum(trimLeadingDot(proto.typeName));
        assert(enumeration !== void 0, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
        field.fieldKind = "enum";
        field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
        field.getDefaultValue = () => {
          return unsafeIsSetExplicit(proto, "defaultValue") ? parseTextFormatEnumValue(enumeration, proto.defaultValue) : void 0;
        };
        break;
      }
      default: {
        field.fieldKind = "scalar";
        field.scalar = type;
        field.longAsString = jstype == JS_STRING;
        field.getDefaultValue = () => {
          return unsafeIsSetExplicit(proto, "defaultValue") ? parseTextFormatScalarValue(type, proto.defaultValue) : void 0;
        };
        break;
      }
    }
    return field;
  }
  function getFileEdition(proto) {
    switch (proto.syntax) {
      case "":
      case "proto2":
        return EDITION_PROTO2;
      case "proto3":
        return EDITION_PROTO3;
      case "editions":
        if (proto.edition in featureDefaults) {
          return proto.edition;
        }
        throw new Error(`${proto.name}: unsupported edition`);
      default:
        throw new Error(`${proto.name}: unsupported syntax "${proto.syntax}"`);
    }
  }
  function findFileDependencies(proto, reg) {
    return proto.dependency.map((wantName) => {
      const dep = reg.getFile(wantName);
      if (!dep) {
        throw new Error(`Cannot find ${wantName}, imported by ${proto.name}`);
      }
      return dep;
    });
  }
  function findEnumSharedPrefix(enumName, values) {
    const prefix = camelToSnakeCase(enumName) + "_";
    for (const value of values) {
      if (!value.name.toLowerCase().startsWith(prefix)) {
        return void 0;
      }
      const shortName = value.name.substring(prefix.length);
      if (shortName.length == 0) {
        return void 0;
      }
      if (/^\d/.test(shortName)) {
        return void 0;
      }
    }
    return prefix;
  }
  function camelToSnakeCase(camel) {
    return (camel.substring(0, 1) + camel.substring(1).replace(/[A-Z]/g, (c) => "_" + c)).toLowerCase();
  }
  function makeTypeName(proto, parent, file) {
    let typeName;
    if (parent) {
      typeName = `${parent.typeName}.${proto.name}`;
    } else if (file.proto.package.length > 0) {
      typeName = `${file.proto.package}.${proto.name}`;
    } else {
      typeName = `${proto.name}`;
    }
    return typeName;
  }
  function trimLeadingDot(typeName) {
    return typeName.startsWith(".") ? typeName.substring(1) : typeName;
  }
  function findOneof(proto, allOneofs) {
    if (!unsafeIsSetExplicit(proto, "oneofIndex")) {
      return void 0;
    }
    if (proto.proto3Optional) {
      return void 0;
    }
    const oneof = allOneofs[proto.oneofIndex];
    assert(oneof, `invalid FieldDescriptorProto: oneof #${proto.oneofIndex} for field #${proto.number} not found`);
    return oneof;
  }
  function getFieldPresence(proto, oneof, isExtension, parent) {
    if (proto.label == LABEL_REQUIRED) {
      return LEGACY_REQUIRED;
    }
    if (proto.label == LABEL_REPEATED) {
      return IMPLICIT2;
    }
    if (!!oneof || proto.proto3Optional) {
      return EXPLICIT;
    }
    if (proto.type == TYPE_MESSAGE) {
      return EXPLICIT;
    }
    if (isExtension) {
      return EXPLICIT;
    }
    return resolveFeature("fieldPresence", { proto, parent });
  }
  function isPackedField(proto, parent) {
    if (proto.label != LABEL_REPEATED) {
      return false;
    }
    switch (proto.type) {
      case TYPE_STRING:
      case TYPE_BYTES:
      case TYPE_GROUP:
      case TYPE_MESSAGE:
        return false;
    }
    const o = proto.options;
    if (o && unsafeIsSetExplicit(o, "packed")) {
      return o.packed;
    }
    return PACKED == resolveFeature("repeatedFieldEncoding", {
      proto,
      parent
    });
  }
  function findMapEntryFields(mapEntry) {
    const key = mapEntry.fields.find((f) => f.number === 1);
    const value = mapEntry.fields.find((f) => f.number === 2);
    assert(key && key.fieldKind == "scalar" && key.scalar != ScalarType.BYTES && key.scalar != ScalarType.FLOAT && key.scalar != ScalarType.DOUBLE && value && value.fieldKind != "list" && value.fieldKind != "map");
    return { key, value };
  }
  function isEnumOpen(desc) {
    var _a;
    return OPEN == resolveFeature("enumType", {
      proto: desc.proto,
      parent: (_a = desc.parent) !== null && _a !== void 0 ? _a : desc.file
    });
  }
  function isDelimitedEncoding(proto, parent) {
    if (proto.type == TYPE_GROUP) {
      return true;
    }
    return DELIMITED == resolveFeature("messageEncoding", {
      proto,
      parent
    });
  }
  function resolveFeature(name, ref) {
    var _a, _b;
    const featureSet = (_a = ref.proto.options) === null || _a === void 0 ? void 0 : _a.features;
    if (featureSet) {
      const val = featureSet[name];
      if (val != 0) {
        return val;
      }
    }
    if ("kind" in ref) {
      if (ref.kind == "message") {
        return resolveFeature(name, (_b = ref.parent) !== null && _b !== void 0 ? _b : ref.file);
      }
      const editionDefaults = featureDefaults[ref.edition];
      if (!editionDefaults) {
        throw new Error(`feature default for edition ${ref.edition} not found`);
      }
      return editionDefaults[name];
    }
    return resolveFeature(name, ref.parent);
  }
  function assert(condition, msg) {
    if (!condition) {
      throw new Error(msg);
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/codegenv1/boot.js
  function boot(boot2) {
    const root = bootFileDescriptorProto(boot2);
    root.messageType.forEach(restoreJsonNames);
    const reg = createFileRegistry(root, () => void 0);
    return reg.getFile(root.name);
  }
  function bootFileDescriptorProto(init) {
    const proto = /* @__PURE__ */ Object.create({
      syntax: "",
      edition: 0
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FileDescriptorProto", dependency: [], publicDependency: [], weakDependency: [], service: [], extension: [] }, init), { messageType: init.messageType.map(bootDescriptorProto), enumType: init.enumType.map(bootEnumDescriptorProto) }));
  }
  function bootDescriptorProto(init) {
    var _a, _b, _c, _d, _e, _f, _g, _h;
    return {
      $typeName: "google.protobuf.DescriptorProto",
      name: init.name,
      field: (_b = (_a = init.field) === null || _a === void 0 ? void 0 : _a.map(bootFieldDescriptorProto)) !== null && _b !== void 0 ? _b : [],
      extension: [],
      nestedType: (_d = (_c = init.nestedType) === null || _c === void 0 ? void 0 : _c.map(bootDescriptorProto)) !== null && _d !== void 0 ? _d : [],
      enumType: (_f = (_e = init.enumType) === null || _e === void 0 ? void 0 : _e.map(bootEnumDescriptorProto)) !== null && _f !== void 0 ? _f : [],
      extensionRange: (_h = (_g = init.extensionRange) === null || _g === void 0 ? void 0 : _g.map((e) => Object.assign({ $typeName: "google.protobuf.DescriptorProto.ExtensionRange" }, e))) !== null && _h !== void 0 ? _h : [],
      oneofDecl: [],
      reservedRange: [],
      reservedName: []
    };
  }
  function bootFieldDescriptorProto(init) {
    const proto = /* @__PURE__ */ Object.create({
      label: 1,
      typeName: "",
      extendee: "",
      defaultValue: "",
      oneofIndex: 0,
      jsonName: "",
      proto3Optional: false
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FieldDescriptorProto" }, init), { options: init.options ? bootFieldOptions(init.options) : void 0 }));
  }
  function bootFieldOptions(init) {
    var _a, _b, _c;
    const proto = /* @__PURE__ */ Object.create({
      ctype: 0,
      packed: false,
      jstype: 0,
      lazy: false,
      unverifiedLazy: false,
      deprecated: false,
      weak: false,
      debugRedact: false,
      retention: 0
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FieldOptions" }, init), { targets: (_a = init.targets) !== null && _a !== void 0 ? _a : [], editionDefaults: (_c = (_b = init.editionDefaults) === null || _b === void 0 ? void 0 : _b.map((e) => Object.assign({ $typeName: "google.protobuf.FieldOptions.EditionDefault" }, e))) !== null && _c !== void 0 ? _c : [], uninterpretedOption: [] }));
  }
  function bootEnumDescriptorProto(init) {
    return {
      $typeName: "google.protobuf.EnumDescriptorProto",
      name: init.name,
      reservedName: [],
      reservedRange: [],
      value: init.value.map((e) => Object.assign({ $typeName: "google.protobuf.EnumValueDescriptorProto" }, e))
    };
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wire/base64-encoding.js
  function base64Decode(base64Str) {
    const table = getDecodeTable();
    let es = base64Str.length * 3 / 4;
    if (base64Str[base64Str.length - 2] == "=")
      es -= 2;
    else if (base64Str[base64Str.length - 1] == "=")
      es -= 1;
    let bytes = new Uint8Array(es), bytePos = 0, groupPos = 0, b, p = 0;
    for (let i = 0; i < base64Str.length; i++) {
      b = table[base64Str.charCodeAt(i)];
      if (b === void 0) {
        switch (base64Str[i]) {
          // @ts-expect-error TS7029: Fallthrough case in switch
          case "=":
            groupPos = 0;
          // reset state when padding found
          case "\n":
          case "\r":
          case "	":
          case " ":
            continue;
          // skip white-space, and padding
          default:
            throw Error("invalid base64 string");
        }
      }
      switch (groupPos) {
        case 0:
          p = b;
          groupPos = 1;
          break;
        case 1:
          bytes[bytePos++] = p << 2 | (b & 48) >> 4;
          p = b;
          groupPos = 2;
          break;
        case 2:
          bytes[bytePos++] = (p & 15) << 4 | (b & 60) >> 2;
          p = b;
          groupPos = 3;
          break;
        case 3:
          bytes[bytePos++] = (p & 3) << 6 | b;
          groupPos = 0;
          break;
      }
    }
    if (groupPos == 1)
      throw Error("invalid base64 string");
    return bytes.subarray(0, bytePos);
  }
  var encodeTableStd;
  var encodeTableUrl;
  var decodeTable;
  function getEncodeTable(encoding) {
    if (!encodeTableStd) {
      encodeTableStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
      encodeTableUrl = encodeTableStd.slice(0, -2).concat("-", "_");
    }
    return encoding == "url" ? (
      // biome-ignore lint/style/noNonNullAssertion: TS fails to narrow down
      encodeTableUrl
    ) : encodeTableStd;
  }
  function getDecodeTable() {
    if (!decodeTable) {
      decodeTable = [];
      const encodeTable = getEncodeTable("std");
      for (let i = 0; i < encodeTable.length; i++)
        decodeTable[encodeTable[i].charCodeAt(0)] = i;
      decodeTable["-".charCodeAt(0)] = encodeTable.indexOf("+");
      decodeTable["_".charCodeAt(0)] = encodeTable.indexOf("/");
    }
    return decodeTable;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/is-message.js
  function isMessage(arg, schema) {
    const isMessage2 = arg !== null && typeof arg == "object" && "$typeName" in arg && typeof arg.$typeName == "string";
    if (!isMessage2) {
      return false;
    }
    if (schema === void 0) {
      return true;
    }
    return schema.typeName === arg.$typeName;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/error.js
  var FieldError = class extends Error {
    constructor(fieldOrOneof, message, name = "FieldValueInvalidError") {
      super(message);
      this.name = name;
      this.field = () => fieldOrOneof;
    }
  };

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/guard.js
  function isObject(arg) {
    return arg !== null && typeof arg == "object" && !Array.isArray(arg);
  }
  function isReflectList(arg, field) {
    var _a, _b, _c, _d;
    if (isObject(arg) && unsafeLocal in arg && "add" in arg && "field" in arg && typeof arg.field == "function") {
      if (field !== void 0) {
        const a = field;
        const b = arg.field();
        return a.listKind == b.listKind && a.scalar === b.scalar && ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) && ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName);
      }
      return true;
    }
    return false;
  }
  function isReflectMap(arg, field) {
    var _a, _b, _c, _d;
    if (isObject(arg) && unsafeLocal in arg && "has" in arg && "field" in arg && typeof arg.field == "function") {
      if (field !== void 0) {
        const a = field, b = arg.field();
        return a.mapKey === b.mapKey && a.mapKind == b.mapKind && a.scalar === b.scalar && ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) && ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName);
      }
      return true;
    }
    return false;
  }
  function isReflectMessage(arg, messageDesc2) {
    return isObject(arg) && unsafeLocal in arg && "desc" in arg && isObject(arg.desc) && arg.desc.kind === "message" && (messageDesc2 === void 0 || arg.desc.typeName == messageDesc2.typeName);
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wire/text-encoding.js
  var symbol = Symbol.for("@bufbuild/protobuf/text-encoding");
  function getTextEncoding() {
    if (globalThis[symbol] == void 0) {
      const te = new globalThis.TextEncoder();
      const td = new globalThis.TextDecoder();
      globalThis[symbol] = {
        encodeUtf8(text) {
          return te.encode(text);
        },
        decodeUtf8(bytes) {
          return td.decode(bytes);
        },
        checkUtf8(text) {
          try {
            encodeURIComponent(text);
            return true;
          } catch (_) {
            return false;
          }
        }
      };
    }
    return globalThis[symbol];
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wire/binary-encoding.js
  var WireType;
  (function(WireType2) {
    WireType2[WireType2["Varint"] = 0] = "Varint";
    WireType2[WireType2["Bit64"] = 1] = "Bit64";
    WireType2[WireType2["LengthDelimited"] = 2] = "LengthDelimited";
    WireType2[WireType2["StartGroup"] = 3] = "StartGroup";
    WireType2[WireType2["EndGroup"] = 4] = "EndGroup";
    WireType2[WireType2["Bit32"] = 5] = "Bit32";
  })(WireType || (WireType = {}));
  var FLOAT32_MAX = 34028234663852886e22;
  var FLOAT32_MIN = -34028234663852886e22;
  var UINT32_MAX = 4294967295;
  var INT32_MAX = 2147483647;
  var INT32_MIN = -2147483648;
  var BinaryReader = class {
    constructor(buf, decodeUtf8 = getTextEncoding().decodeUtf8) {
      this.decodeUtf8 = decodeUtf8;
      this.varint64 = varint64read;
      this.uint32 = varint32read;
      this.buf = buf;
      this.len = buf.length;
      this.pos = 0;
      this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    }
    /**
     * Reads a tag - field number and wire type.
     */
    tag() {
      let tag = this.uint32(), fieldNo = tag >>> 3, wireType = tag & 7;
      if (fieldNo <= 0 || wireType < 0 || wireType > 5)
        throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
      return [fieldNo, wireType];
    }
    /**
     * Skip one element and return the skipped data.
     *
     * When skipping StartGroup, provide the tags field number to check for
     * matching field number in the EndGroup tag.
     */
    skip(wireType, fieldNo) {
      let start = this.pos;
      switch (wireType) {
        case WireType.Varint:
          while (this.buf[this.pos++] & 128) {
          }
          break;
        // @ts-expect-error TS7029: Fallthrough case in switch
        case WireType.Bit64:
          this.pos += 4;
        case WireType.Bit32:
          this.pos += 4;
          break;
        case WireType.LengthDelimited:
          let len = this.uint32();
          this.pos += len;
          break;
        case WireType.StartGroup:
          for (; ; ) {
            const [fn, wt] = this.tag();
            if (wt === WireType.EndGroup) {
              if (fieldNo !== void 0 && fn !== fieldNo) {
                throw new Error("invalid end group tag");
              }
              break;
            }
            this.skip(wt, fn);
          }
          break;
        default:
          throw new Error("cant skip wire type " + wireType);
      }
      this.assertBounds();
      return this.buf.subarray(start, this.pos);
    }
    /**
     * Throws error if position in byte array is out of range.
     */
    assertBounds() {
      if (this.pos > this.len)
        throw new RangeError("premature EOF");
    }
    /**
     * Read a `int32` field, a signed 32 bit varint.
     */
    int32() {
      return this.uint32() | 0;
    }
    /**
     * Read a `sint32` field, a signed, zigzag-encoded 32-bit varint.
     */
    sint32() {
      let zze = this.uint32();
      return zze >>> 1 ^ -(zze & 1);
    }
    /**
     * Read a `int64` field, a signed 64-bit varint.
     */
    int64() {
      return protoInt64.dec(...this.varint64());
    }
    /**
     * Read a `uint64` field, an unsigned 64-bit varint.
     */
    uint64() {
      return protoInt64.uDec(...this.varint64());
    }
    /**
     * Read a `sint64` field, a signed, zig-zag-encoded 64-bit varint.
     */
    sint64() {
      let [lo, hi] = this.varint64();
      let s = -(lo & 1);
      lo = (lo >>> 1 | (hi & 1) << 31) ^ s;
      hi = hi >>> 1 ^ s;
      return protoInt64.dec(lo, hi);
    }
    /**
     * Read a `bool` field, a variant.
     */
    bool() {
      let [lo, hi] = this.varint64();
      return lo !== 0 || hi !== 0;
    }
    /**
     * Read a `fixed32` field, an unsigned, fixed-length 32-bit integer.
     */
    fixed32() {
      return this.view.getUint32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `sfixed32` field, a signed, fixed-length 32-bit integer.
     */
    sfixed32() {
      return this.view.getInt32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `fixed64` field, an unsigned, fixed-length 64 bit integer.
     */
    fixed64() {
      return protoInt64.uDec(this.sfixed32(), this.sfixed32());
    }
    /**
     * Read a `fixed64` field, a signed, fixed-length 64-bit integer.
     */
    sfixed64() {
      return protoInt64.dec(this.sfixed32(), this.sfixed32());
    }
    /**
     * Read a `float` field, 32-bit floating point number.
     */
    float() {
      return this.view.getFloat32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `double` field, a 64-bit floating point number.
     */
    double() {
      return this.view.getFloat64((this.pos += 8) - 8, true);
    }
    /**
     * Read a `bytes` field, length-delimited arbitrary data.
     */
    bytes() {
      let len = this.uint32(), start = this.pos;
      this.pos += len;
      this.assertBounds();
      return this.buf.subarray(start, start + len);
    }
    /**
     * Read a `string` field, length-delimited data converted to UTF-8 text.
     */
    string() {
      return this.decodeUtf8(this.bytes());
    }
  };

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect-check.js
  function checkField(field, value) {
    const check = field.fieldKind == "list" ? isReflectList(value, field) : field.fieldKind == "map" ? isReflectMap(value, field) : checkSingular(field, value);
    if (check === true) {
      return void 0;
    }
    let reason;
    switch (field.fieldKind) {
      case "list":
        reason = `expected ${formatReflectList(field)}, got ${formatVal(value)}`;
        break;
      case "map":
        reason = `expected ${formatReflectMap(field)}, got ${formatVal(value)}`;
        break;
      default: {
        reason = reasonSingular(field, value, check);
      }
    }
    return new FieldError(field, reason);
  }
  function checkListItem(field, index, value) {
    const check = checkSingular(field, value);
    if (check !== true) {
      return new FieldError(field, `list item #${index + 1}: ${reasonSingular(field, value, check)}`);
    }
    return void 0;
  }
  function checkMapEntry(field, key, value) {
    const checkKey = checkScalarValue(key, field.mapKey);
    if (checkKey !== true) {
      return new FieldError(field, `invalid map key: ${reasonSingular({ scalar: field.mapKey }, key, checkKey)}`);
    }
    const checkVal = checkSingular(field, value);
    if (checkVal !== true) {
      return new FieldError(field, `map entry ${formatVal(key)}: ${reasonSingular(field, value, checkVal)}`);
    }
    return void 0;
  }
  function checkSingular(field, value) {
    if (field.scalar !== void 0) {
      return checkScalarValue(value, field.scalar);
    }
    if (field.enum !== void 0) {
      if (field.enum.open) {
        return Number.isInteger(value);
      }
      return field.enum.values.some((v) => v.number === value);
    }
    return isReflectMessage(value, field.message);
  }
  function checkScalarValue(value, scalar) {
    switch (scalar) {
      case ScalarType.DOUBLE:
        return typeof value == "number";
      case ScalarType.FLOAT:
        if (typeof value != "number") {
          return false;
        }
        if (Number.isNaN(value) || !Number.isFinite(value)) {
          return true;
        }
        if (value > FLOAT32_MAX || value < FLOAT32_MIN) {
          return `${value.toFixed()} out of range`;
        }
        return true;
      case ScalarType.INT32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32:
        if (typeof value !== "number" || !Number.isInteger(value)) {
          return false;
        }
        if (value > INT32_MAX || value < INT32_MIN) {
          return `${value.toFixed()} out of range`;
        }
        return true;
      case ScalarType.FIXED32:
      case ScalarType.UINT32:
        if (typeof value !== "number" || !Number.isInteger(value)) {
          return false;
        }
        if (value > UINT32_MAX || value < 0) {
          return `${value.toFixed()} out of range`;
        }
        return true;
      case ScalarType.BOOL:
        return typeof value == "boolean";
      case ScalarType.STRING:
        if (typeof value != "string") {
          return false;
        }
        return getTextEncoding().checkUtf8(value) || "invalid UTF8";
      case ScalarType.BYTES:
        return value instanceof Uint8Array;
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        if (typeof value == "bigint" || typeof value == "number" || typeof value == "string" && value.length > 0) {
          try {
            protoInt64.parse(value);
            return true;
          } catch (_) {
            return `${value} out of range`;
          }
        }
        return false;
      case ScalarType.FIXED64:
      case ScalarType.UINT64:
        if (typeof value == "bigint" || typeof value == "number" || typeof value == "string" && value.length > 0) {
          try {
            protoInt64.uParse(value);
            return true;
          } catch (_) {
            return `${value} out of range`;
          }
        }
        return false;
    }
  }
  function reasonSingular(field, val, details) {
    details = typeof details == "string" ? `: ${details}` : `, got ${formatVal(val)}`;
    if (field.scalar !== void 0) {
      return `expected ${scalarTypeDescription(field.scalar)}` + details;
    }
    if (field.enum !== void 0) {
      return `expected ${field.enum.toString()}` + details;
    }
    return `expected ${formatReflectMessage(field.message)}` + details;
  }
  function formatVal(val) {
    switch (typeof val) {
      case "object":
        if (val === null) {
          return "null";
        }
        if (val instanceof Uint8Array) {
          return `Uint8Array(${val.length})`;
        }
        if (Array.isArray(val)) {
          return `Array(${val.length})`;
        }
        if (isReflectList(val)) {
          return formatReflectList(val.field());
        }
        if (isReflectMap(val)) {
          return formatReflectMap(val.field());
        }
        if (isReflectMessage(val)) {
          return formatReflectMessage(val.desc);
        }
        if (isMessage(val)) {
          return `message ${val.$typeName}`;
        }
        return "object";
      case "string":
        return val.length > 30 ? "string" : `"${val.split('"').join('\\"')}"`;
      case "boolean":
        return String(val);
      case "number":
        return String(val);
      case "bigint":
        return String(val) + "n";
      default:
        return typeof val;
    }
  }
  function formatReflectMessage(desc) {
    return `ReflectMessage (${desc.typeName})`;
  }
  function formatReflectList(field) {
    switch (field.listKind) {
      case "message":
        return `ReflectList (${field.message.toString()})`;
      case "enum":
        return `ReflectList (${field.enum.toString()})`;
      case "scalar":
        return `ReflectList (${ScalarType[field.scalar]})`;
    }
  }
  function formatReflectMap(field) {
    switch (field.mapKind) {
      case "message":
        return `ReflectMap (${ScalarType[field.mapKey]}, ${field.message.toString()})`;
      case "enum":
        return `ReflectMap (${ScalarType[field.mapKey]}, ${field.enum.toString()})`;
      case "scalar":
        return `ReflectMap (${ScalarType[field.mapKey]}, ${ScalarType[field.scalar]})`;
    }
  }
  function scalarTypeDescription(scalar) {
    switch (scalar) {
      case ScalarType.STRING:
        return "string";
      case ScalarType.BOOL:
        return "boolean";
      case ScalarType.INT64:
      case ScalarType.SINT64:
      case ScalarType.SFIXED64:
        return "bigint (int64)";
      case ScalarType.UINT64:
      case ScalarType.FIXED64:
        return "bigint (uint64)";
      case ScalarType.BYTES:
        return "Uint8Array";
      case ScalarType.DOUBLE:
        return "number (float64)";
      case ScalarType.FLOAT:
        return "number (float32)";
      case ScalarType.FIXED32:
      case ScalarType.UINT32:
        return "number (uint32)";
      case ScalarType.INT32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32:
        return "number (int32)";
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wkt/wrappers.js
  function isWrapper(arg) {
    return isWrapperTypeName(arg.$typeName);
  }
  function isWrapperDesc(messageDesc2) {
    const f = messageDesc2.fields[0];
    return isWrapperTypeName(messageDesc2.typeName) && f !== void 0 && f.fieldKind == "scalar" && f.name == "value" && f.number == 1;
  }
  function isWrapperTypeName(name) {
    return name.startsWith("google.protobuf.") && [
      "DoubleValue",
      "FloatValue",
      "Int64Value",
      "UInt64Value",
      "Int32Value",
      "UInt32Value",
      "BoolValue",
      "StringValue",
      "BytesValue"
    ].includes(name.substring(16));
  }

  // node_modules/@bufbuild/protobuf/dist/esm/create.js
  var EDITION_PROTO32 = 999;
  var EDITION_PROTO22 = 998;
  var IMPLICIT3 = 2;
  function create(schema, init) {
    if (isMessage(init, schema)) {
      return init;
    }
    const message = createZeroMessage(schema);
    if (init !== void 0) {
      initMessage(schema, message, init);
    }
    return message;
  }
  function initMessage(messageDesc2, message, init) {
    for (const member of messageDesc2.members) {
      let value = init[member.localName];
      if (value == null) {
        continue;
      }
      let field;
      if (member.kind == "oneof") {
        const oneofField = unsafeOneofCase(init, member);
        if (!oneofField) {
          continue;
        }
        field = oneofField;
        value = unsafeGet(init, oneofField);
      } else {
        field = member;
      }
      switch (field.fieldKind) {
        case "message":
          value = toMessage(field, value);
          break;
        case "scalar":
          value = initScalar(field, value);
          break;
        case "list":
          value = initList(field, value);
          break;
        case "map":
          value = initMap(field, value);
          break;
      }
      unsafeSet(message, field, value);
    }
    return message;
  }
  function initScalar(field, value) {
    if (field.scalar == ScalarType.BYTES) {
      return toU8Arr(value);
    }
    return value;
  }
  function initMap(field, value) {
    if (isObject(value)) {
      if (field.scalar == ScalarType.BYTES) {
        return convertObjectValues(value, toU8Arr);
      }
      if (field.mapKind == "message") {
        return convertObjectValues(value, (val) => toMessage(field, val));
      }
    }
    return value;
  }
  function initList(field, value) {
    if (Array.isArray(value)) {
      if (field.scalar == ScalarType.BYTES) {
        return value.map(toU8Arr);
      }
      if (field.listKind == "message") {
        return value.map((item) => toMessage(field, item));
      }
    }
    return value;
  }
  function toMessage(field, value) {
    if (field.fieldKind == "message" && !field.oneof && isWrapperDesc(field.message)) {
      return initScalar(field.message.fields[0], value);
    }
    if (isObject(value)) {
      if (field.message.typeName == "google.protobuf.Struct" && field.parent.typeName !== "google.protobuf.Value") {
        return value;
      }
      if (!isMessage(value, field.message)) {
        return create(field.message, value);
      }
    }
    return value;
  }
  function toU8Arr(value) {
    return Array.isArray(value) ? new Uint8Array(value) : value;
  }
  function convertObjectValues(obj, fn) {
    const ret = {};
    for (const entry of Object.entries(obj)) {
      ret[entry[0]] = fn(entry[1]);
    }
    return ret;
  }
  var tokenZeroMessageField = Symbol();
  var messagePrototypes = /* @__PURE__ */ new WeakMap();
  function createZeroMessage(desc) {
    let msg;
    if (!needsPrototypeChain(desc)) {
      msg = {
        $typeName: desc.typeName
      };
      for (const member of desc.members) {
        if (member.kind == "oneof" || member.presence == IMPLICIT3) {
          msg[member.localName] = createZeroField(member);
        }
      }
    } else {
      const cached = messagePrototypes.get(desc);
      let prototype;
      let members;
      if (cached) {
        ({ prototype, members } = cached);
      } else {
        prototype = {};
        members = /* @__PURE__ */ new Set();
        for (const member of desc.members) {
          if (member.kind == "oneof") {
            continue;
          }
          if (member.fieldKind != "scalar" && member.fieldKind != "enum") {
            continue;
          }
          if (member.presence == IMPLICIT3) {
            continue;
          }
          members.add(member);
          prototype[member.localName] = createZeroField(member);
        }
        messagePrototypes.set(desc, { prototype, members });
      }
      msg = Object.create(prototype);
      msg.$typeName = desc.typeName;
      for (const member of desc.members) {
        if (members.has(member)) {
          continue;
        }
        if (member.kind == "field") {
          if (member.fieldKind == "message") {
            continue;
          }
          if (member.fieldKind == "scalar" || member.fieldKind == "enum") {
            if (member.presence != IMPLICIT3) {
              continue;
            }
          }
        }
        msg[member.localName] = createZeroField(member);
      }
    }
    return msg;
  }
  function needsPrototypeChain(desc) {
    switch (desc.file.edition) {
      case EDITION_PROTO32:
        return false;
      case EDITION_PROTO22:
        return true;
      default:
        return desc.fields.some((f) => f.presence != IMPLICIT3 && f.fieldKind != "message" && !f.oneof);
    }
  }
  function createZeroField(field) {
    if (field.kind == "oneof") {
      return { case: void 0 };
    }
    if (field.fieldKind == "list") {
      return [];
    }
    if (field.fieldKind == "map") {
      return {};
    }
    if (field.fieldKind == "message") {
      return tokenZeroMessageField;
    }
    const defaultValue = field.getDefaultValue();
    if (defaultValue !== void 0) {
      return field.fieldKind == "scalar" && field.longAsString ? defaultValue.toString() : defaultValue;
    }
    return field.fieldKind == "scalar" ? scalarZeroValue(field.scalar, field.longAsString) : field.enum.values[0].number;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect.js
  function reflect(messageDesc2, message, check = true) {
    return new ReflectMessageImpl(messageDesc2, message, check);
  }
  var ReflectMessageImpl = class {
    get sortedFields() {
      var _a;
      return (_a = this._sortedFields) !== null && _a !== void 0 ? _a : (
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        this._sortedFields = this.desc.fields.concat().sort((a, b) => a.number - b.number)
      );
    }
    constructor(messageDesc2, message, check = true) {
      this.lists = /* @__PURE__ */ new Map();
      this.maps = /* @__PURE__ */ new Map();
      this.check = check;
      this.desc = messageDesc2;
      this.message = this[unsafeLocal] = message !== null && message !== void 0 ? message : create(messageDesc2);
      this.fields = messageDesc2.fields;
      this.oneofs = messageDesc2.oneofs;
      this.members = messageDesc2.members;
    }
    findNumber(number) {
      if (!this._fieldsByNumber) {
        this._fieldsByNumber = new Map(this.desc.fields.map((f) => [f.number, f]));
      }
      return this._fieldsByNumber.get(number);
    }
    oneofCase(oneof) {
      assertOwn(this.message, oneof);
      return unsafeOneofCase(this.message, oneof);
    }
    isSet(field) {
      assertOwn(this.message, field);
      return unsafeIsSet(this.message, field);
    }
    clear(field) {
      assertOwn(this.message, field);
      unsafeClear(this.message, field);
    }
    get(field) {
      assertOwn(this.message, field);
      const value = unsafeGet(this.message, field);
      switch (field.fieldKind) {
        case "list":
          let list = this.lists.get(field);
          if (!list || list[unsafeLocal] !== value) {
            this.lists.set(
              field,
              // biome-ignore lint/suspicious/noAssignInExpressions: no
              list = new ReflectListImpl(field, value, this.check)
            );
          }
          return list;
        case "map":
          let map = this.maps.get(field);
          if (!map || map[unsafeLocal] !== value) {
            this.maps.set(
              field,
              // biome-ignore lint/suspicious/noAssignInExpressions: no
              map = new ReflectMapImpl(field, value, this.check)
            );
          }
          return map;
        case "message":
          return messageToReflect(field, value, this.check);
        case "scalar":
          return value === void 0 ? scalarZeroValue(field.scalar, false) : longToReflect(field, value);
        case "enum":
          return value !== null && value !== void 0 ? value : field.enum.values[0].number;
      }
    }
    set(field, value) {
      assertOwn(this.message, field);
      if (this.check) {
        const err = checkField(field, value);
        if (err) {
          throw err;
        }
      }
      let local;
      if (field.fieldKind == "message") {
        local = messageToLocal(field, value);
      } else if (isReflectMap(value) || isReflectList(value)) {
        local = value[unsafeLocal];
      } else {
        local = longToLocal(field, value);
      }
      unsafeSet(this.message, field, local);
    }
    getUnknown() {
      return this.message.$unknown;
    }
    setUnknown(value) {
      this.message.$unknown = value;
    }
  };
  function assertOwn(owner, member) {
    if (member.parent.typeName !== owner.$typeName) {
      throw new FieldError(member, `cannot use ${member.toString()} with message ${owner.$typeName}`, "ForeignFieldError");
    }
  }
  var ReflectListImpl = class {
    field() {
      return this._field;
    }
    get size() {
      return this._arr.length;
    }
    constructor(field, unsafeInput, check) {
      this._field = field;
      this._arr = this[unsafeLocal] = unsafeInput;
      this.check = check;
    }
    get(index) {
      const item = this._arr[index];
      return item === void 0 ? void 0 : listItemToReflect(this._field, item, this.check);
    }
    set(index, item) {
      if (index < 0 || index >= this._arr.length) {
        throw new FieldError(this._field, `list item #${index + 1}: out of range`);
      }
      if (this.check) {
        const err = checkListItem(this._field, index, item);
        if (err) {
          throw err;
        }
      }
      this._arr[index] = listItemToLocal(this._field, item);
    }
    add(item) {
      if (this.check) {
        const err = checkListItem(this._field, this._arr.length, item);
        if (err) {
          throw err;
        }
      }
      this._arr.push(listItemToLocal(this._field, item));
      return void 0;
    }
    clear() {
      this._arr.splice(0, this._arr.length);
    }
    [Symbol.iterator]() {
      return this.values();
    }
    keys() {
      return this._arr.keys();
    }
    *values() {
      for (const item of this._arr) {
        yield listItemToReflect(this._field, item, this.check);
      }
    }
    *entries() {
      for (let i = 0; i < this._arr.length; i++) {
        yield [i, listItemToReflect(this._field, this._arr[i], this.check)];
      }
    }
  };
  var ReflectMapImpl = class {
    constructor(field, unsafeInput, check = true) {
      this.obj = this[unsafeLocal] = unsafeInput !== null && unsafeInput !== void 0 ? unsafeInput : {};
      this.check = check;
      this._field = field;
    }
    field() {
      return this._field;
    }
    set(key, value) {
      if (this.check) {
        const err = checkMapEntry(this._field, key, value);
        if (err) {
          throw err;
        }
      }
      this.obj[mapKeyToLocal(key)] = mapValueToLocal(this._field, value);
      return this;
    }
    delete(key) {
      const k = mapKeyToLocal(key);
      const has = Object.prototype.hasOwnProperty.call(this.obj, k);
      if (has) {
        delete this.obj[k];
      }
      return has;
    }
    clear() {
      for (const key of Object.keys(this.obj)) {
        delete this.obj[key];
      }
    }
    get(key) {
      let val = this.obj[mapKeyToLocal(key)];
      if (val !== void 0) {
        val = mapValueToReflect(this._field, val, this.check);
      }
      return val;
    }
    has(key) {
      return Object.prototype.hasOwnProperty.call(this.obj, mapKeyToLocal(key));
    }
    *keys() {
      for (const objKey of Object.keys(this.obj)) {
        yield mapKeyToReflect(objKey, this._field.mapKey);
      }
    }
    *entries() {
      for (const objEntry of Object.entries(this.obj)) {
        yield [
          mapKeyToReflect(objEntry[0], this._field.mapKey),
          mapValueToReflect(this._field, objEntry[1], this.check)
        ];
      }
    }
    [Symbol.iterator]() {
      return this.entries();
    }
    get size() {
      return Object.keys(this.obj).length;
    }
    *values() {
      for (const val of Object.values(this.obj)) {
        yield mapValueToReflect(this._field, val, this.check);
      }
    }
    forEach(callbackfn, thisArg) {
      for (const mapEntry of this.entries()) {
        callbackfn.call(thisArg, mapEntry[1], mapEntry[0], this);
      }
    }
  };
  function messageToLocal(field, value) {
    if (!isReflectMessage(value)) {
      return value;
    }
    if (isWrapper(value.message) && !field.oneof && field.fieldKind == "message") {
      return value.message.value;
    }
    if (value.desc.typeName == "google.protobuf.Struct" && field.parent.typeName != "google.protobuf.Value") {
      return wktStructToLocal(value.message);
    }
    return value.message;
  }
  function messageToReflect(field, value, check) {
    if (value !== void 0) {
      if (isWrapperDesc(field.message) && !field.oneof && field.fieldKind == "message") {
        value = {
          $typeName: field.message.typeName,
          value: longToReflect(field.message.fields[0], value)
        };
      } else if (field.message.typeName == "google.protobuf.Struct" && field.parent.typeName != "google.protobuf.Value" && isObject(value)) {
        value = wktStructToReflect(value);
      }
    }
    return new ReflectMessageImpl(field.message, value, check);
  }
  function listItemToLocal(field, value) {
    if (field.listKind == "message") {
      return messageToLocal(field, value);
    }
    return longToLocal(field, value);
  }
  function listItemToReflect(field, value, check) {
    if (field.listKind == "message") {
      return messageToReflect(field, value, check);
    }
    return longToReflect(field, value);
  }
  function mapValueToLocal(field, value) {
    if (field.mapKind == "message") {
      return messageToLocal(field, value);
    }
    return longToLocal(field, value);
  }
  function mapValueToReflect(field, value, check) {
    if (field.mapKind == "message") {
      return messageToReflect(field, value, check);
    }
    return value;
  }
  function mapKeyToLocal(key) {
    return typeof key == "string" || typeof key == "number" ? key : String(key);
  }
  function mapKeyToReflect(key, type) {
    switch (type) {
      case ScalarType.STRING:
        return key;
      case ScalarType.INT32:
      case ScalarType.FIXED32:
      case ScalarType.UINT32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32: {
        const n = Number.parseInt(key);
        if (Number.isFinite(n)) {
          return n;
        }
        break;
      }
      case ScalarType.BOOL:
        switch (key) {
          case "true":
            return true;
          case "false":
            return false;
        }
        break;
      case ScalarType.UINT64:
      case ScalarType.FIXED64:
        try {
          return protoInt64.uParse(key);
        } catch (_a) {
        }
        break;
      default:
        try {
          return protoInt64.parse(key);
        } catch (_b) {
        }
        break;
    }
    return key;
  }
  function longToReflect(field, value) {
    switch (field.scalar) {
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        if ("longAsString" in field && field.longAsString && typeof value == "string") {
          value = protoInt64.parse(value);
        }
        break;
      case ScalarType.FIXED64:
      case ScalarType.UINT64:
        if ("longAsString" in field && field.longAsString && typeof value == "string") {
          value = protoInt64.uParse(value);
        }
        break;
    }
    return value;
  }
  function longToLocal(field, value) {
    switch (field.scalar) {
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        if ("longAsString" in field && field.longAsString) {
          value = String(value);
        } else if (typeof value == "string" || typeof value == "number") {
          value = protoInt64.parse(value);
        }
        break;
      case ScalarType.FIXED64:
      case ScalarType.UINT64:
        if ("longAsString" in field && field.longAsString) {
          value = String(value);
        } else if (typeof value == "string" || typeof value == "number") {
          value = protoInt64.uParse(value);
        }
        break;
    }
    return value;
  }
  function wktStructToReflect(json) {
    const struct = {
      $typeName: "google.protobuf.Struct",
      fields: {}
    };
    if (isObject(json)) {
      for (const [k, v] of Object.entries(json)) {
        struct.fields[k] = wktValueToReflect(v);
      }
    }
    return struct;
  }
  function wktStructToLocal(val) {
    const json = {};
    for (const [k, v] of Object.entries(val.fields)) {
      json[k] = wktValueToLocal(v);
    }
    return json;
  }
  function wktValueToLocal(val) {
    switch (val.kind.case) {
      case "structValue":
        return wktStructToLocal(val.kind.value);
      case "listValue":
        return val.kind.value.values.map(wktValueToLocal);
      case "nullValue":
      case void 0:
        return null;
      default:
        return val.kind.value;
    }
  }
  function wktValueToReflect(json) {
    const value = {
      $typeName: "google.protobuf.Value",
      kind: { case: void 0 }
    };
    switch (typeof json) {
      case "number":
        value.kind = { case: "numberValue", value: json };
        break;
      case "string":
        value.kind = { case: "stringValue", value: json };
        break;
      case "boolean":
        value.kind = { case: "boolValue", value: json };
        break;
      case "object":
        if (json === null) {
          const nullValue = 0;
          value.kind = { case: "nullValue", value: nullValue };
        } else if (Array.isArray(json)) {
          const listValue = {
            $typeName: "google.protobuf.ListValue",
            values: []
          };
          if (Array.isArray(json)) {
            for (const e of json) {
              listValue.values.push(wktValueToReflect(e));
            }
          }
          value.kind = {
            case: "listValue",
            value: listValue
          };
        } else {
          value.kind = {
            case: "structValue",
            value: wktStructToReflect(json)
          };
        }
        break;
    }
    return value;
  }

  // node_modules/@bufbuild/protobuf/dist/esm/codegenv1/message.js
  function messageDesc(file, path, ...paths) {
    return paths.reduce((acc, cur) => acc.nestedMessages[cur], file.messages[path]);
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/descriptor_pb.js
  var file_google_protobuf_descriptor = /* @__PURE__ */ boot({ "name": "google/protobuf/descriptor.proto", "package": "google.protobuf", "messageType": [{ "name": "FileDescriptorSet", "field": [{ "name": "file", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.FileDescriptorProto" }], "extensionRange": [{ "start": 536e6, "end": 536000001 }] }, { "name": "FileDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "package", "number": 2, "type": 9, "label": 1 }, { "name": "dependency", "number": 3, "type": 9, "label": 3 }, { "name": "public_dependency", "number": 10, "type": 5, "label": 3 }, { "name": "weak_dependency", "number": 11, "type": 5, "label": 3 }, { "name": "message_type", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto" }, { "name": "enum_type", "number": 5, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto" }, { "name": "service", "number": 6, "type": 11, "label": 3, "typeName": ".google.protobuf.ServiceDescriptorProto" }, { "name": "extension", "number": 7, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "options", "number": 8, "type": 11, "label": 1, "typeName": ".google.protobuf.FileOptions" }, { "name": "source_code_info", "number": 9, "type": 11, "label": 1, "typeName": ".google.protobuf.SourceCodeInfo" }, { "name": "syntax", "number": 12, "type": 9, "label": 1 }, { "name": "edition", "number": 14, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }] }, { "name": "DescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "field", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "extension", "number": 6, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "nested_type", "number": 3, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto" }, { "name": "enum_type", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto" }, { "name": "extension_range", "number": 5, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto.ExtensionRange" }, { "name": "oneof_decl", "number": 8, "type": 11, "label": 3, "typeName": ".google.protobuf.OneofDescriptorProto" }, { "name": "options", "number": 7, "type": 11, "label": 1, "typeName": ".google.protobuf.MessageOptions" }, { "name": "reserved_range", "number": 9, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto.ReservedRange" }, { "name": "reserved_name", "number": 10, "type": 9, "label": 3 }], "nestedType": [{ "name": "ExtensionRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.ExtensionRangeOptions" }] }, { "name": "ReservedRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }] }] }, { "name": "ExtensionRangeOptions", "field": [{ "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }, { "name": "declaration", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.ExtensionRangeOptions.Declaration", "options": { "retention": 2 } }, { "name": "features", "number": 50, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "verification", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.ExtensionRangeOptions.VerificationState", "defaultValue": "UNVERIFIED", "options": { "retention": 2 } }], "nestedType": [{ "name": "Declaration", "field": [{ "name": "number", "number": 1, "type": 5, "label": 1 }, { "name": "full_name", "number": 2, "type": 9, "label": 1 }, { "name": "type", "number": 3, "type": 9, "label": 1 }, { "name": "reserved", "number": 5, "type": 8, "label": 1 }, { "name": "repeated", "number": 6, "type": 8, "label": 1 }] }], "enumType": [{ "name": "VerificationState", "value": [{ "name": "DECLARATION", "number": 0 }, { "name": "UNVERIFIED", "number": 1 }] }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "FieldDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "number", "number": 3, "type": 5, "label": 1 }, { "name": "label", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldDescriptorProto.Label" }, { "name": "type", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldDescriptorProto.Type" }, { "name": "type_name", "number": 6, "type": 9, "label": 1 }, { "name": "extendee", "number": 2, "type": 9, "label": 1 }, { "name": "default_value", "number": 7, "type": 9, "label": 1 }, { "name": "oneof_index", "number": 9, "type": 5, "label": 1 }, { "name": "json_name", "number": 10, "type": 9, "label": 1 }, { "name": "options", "number": 8, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions" }, { "name": "proto3_optional", "number": 17, "type": 8, "label": 1 }], "enumType": [{ "name": "Type", "value": [{ "name": "TYPE_DOUBLE", "number": 1 }, { "name": "TYPE_FLOAT", "number": 2 }, { "name": "TYPE_INT64", "number": 3 }, { "name": "TYPE_UINT64", "number": 4 }, { "name": "TYPE_INT32", "number": 5 }, { "name": "TYPE_FIXED64", "number": 6 }, { "name": "TYPE_FIXED32", "number": 7 }, { "name": "TYPE_BOOL", "number": 8 }, { "name": "TYPE_STRING", "number": 9 }, { "name": "TYPE_GROUP", "number": 10 }, { "name": "TYPE_MESSAGE", "number": 11 }, { "name": "TYPE_BYTES", "number": 12 }, { "name": "TYPE_UINT32", "number": 13 }, { "name": "TYPE_ENUM", "number": 14 }, { "name": "TYPE_SFIXED32", "number": 15 }, { "name": "TYPE_SFIXED64", "number": 16 }, { "name": "TYPE_SINT32", "number": 17 }, { "name": "TYPE_SINT64", "number": 18 }] }, { "name": "Label", "value": [{ "name": "LABEL_OPTIONAL", "number": 1 }, { "name": "LABEL_REPEATED", "number": 3 }, { "name": "LABEL_REQUIRED", "number": 2 }] }] }, { "name": "OneofDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "options", "number": 2, "type": 11, "label": 1, "typeName": ".google.protobuf.OneofOptions" }] }, { "name": "EnumDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "value", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumValueDescriptorProto" }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.EnumOptions" }, { "name": "reserved_range", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto.EnumReservedRange" }, { "name": "reserved_name", "number": 5, "type": 9, "label": 3 }], "nestedType": [{ "name": "EnumReservedRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }] }] }, { "name": "EnumValueDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "number", "number": 2, "type": 5, "label": 1 }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.EnumValueOptions" }] }, { "name": "ServiceDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "method", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.MethodDescriptorProto" }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.ServiceOptions" }] }, { "name": "MethodDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "input_type", "number": 2, "type": 9, "label": 1 }, { "name": "output_type", "number": 3, "type": 9, "label": 1 }, { "name": "options", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.MethodOptions" }, { "name": "client_streaming", "number": 5, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "server_streaming", "number": 6, "type": 8, "label": 1, "defaultValue": "false" }] }, { "name": "FileOptions", "field": [{ "name": "java_package", "number": 1, "type": 9, "label": 1 }, { "name": "java_outer_classname", "number": 8, "type": 9, "label": 1 }, { "name": "java_multiple_files", "number": 10, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "java_generate_equals_and_hash", "number": 20, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "java_string_check_utf8", "number": 27, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "optimize_for", "number": 9, "type": 14, "label": 1, "typeName": ".google.protobuf.FileOptions.OptimizeMode", "defaultValue": "SPEED" }, { "name": "go_package", "number": 11, "type": 9, "label": 1 }, { "name": "cc_generic_services", "number": 16, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "java_generic_services", "number": 17, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "py_generic_services", "number": 18, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 23, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "cc_enable_arenas", "number": 31, "type": 8, "label": 1, "defaultValue": "true" }, { "name": "objc_class_prefix", "number": 36, "type": 9, "label": 1 }, { "name": "csharp_namespace", "number": 37, "type": 9, "label": 1 }, { "name": "swift_prefix", "number": 39, "type": 9, "label": 1 }, { "name": "php_class_prefix", "number": 40, "type": 9, "label": 1 }, { "name": "php_namespace", "number": 41, "type": 9, "label": 1 }, { "name": "php_metadata_namespace", "number": 44, "type": 9, "label": 1 }, { "name": "ruby_package", "number": 45, "type": 9, "label": 1 }, { "name": "features", "number": 50, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "enumType": [{ "name": "OptimizeMode", "value": [{ "name": "SPEED", "number": 1 }, { "name": "CODE_SIZE", "number": 2 }, { "name": "LITE_RUNTIME", "number": 3 }] }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "MessageOptions", "field": [{ "name": "message_set_wire_format", "number": 1, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "no_standard_descriptor_accessor", "number": 2, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "map_entry", "number": 7, "type": 8, "label": 1 }, { "name": "deprecated_legacy_json_field_conflicts", "number": 11, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "features", "number": 12, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "FieldOptions", "field": [{ "name": "ctype", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.CType", "defaultValue": "STRING" }, { "name": "packed", "number": 2, "type": 8, "label": 1 }, { "name": "jstype", "number": 6, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.JSType", "defaultValue": "JS_NORMAL" }, { "name": "lazy", "number": 5, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "unverified_lazy", "number": 15, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "weak", "number": 10, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "debug_redact", "number": 16, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "retention", "number": 17, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.OptionRetention" }, { "name": "targets", "number": 19, "type": 14, "label": 3, "typeName": ".google.protobuf.FieldOptions.OptionTargetType" }, { "name": "edition_defaults", "number": 20, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldOptions.EditionDefault" }, { "name": "features", "number": 21, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "feature_support", "number": 22, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions.FeatureSupport" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "nestedType": [{ "name": "EditionDefault", "field": [{ "name": "edition", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "value", "number": 2, "type": 9, "label": 1 }] }, { "name": "FeatureSupport", "field": [{ "name": "edition_introduced", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "edition_deprecated", "number": 2, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "deprecation_warning", "number": 3, "type": 9, "label": 1 }, { "name": "edition_removed", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }] }], "enumType": [{ "name": "CType", "value": [{ "name": "STRING", "number": 0 }, { "name": "CORD", "number": 1 }, { "name": "STRING_PIECE", "number": 2 }] }, { "name": "JSType", "value": [{ "name": "JS_NORMAL", "number": 0 }, { "name": "JS_STRING", "number": 1 }, { "name": "JS_NUMBER", "number": 2 }] }, { "name": "OptionRetention", "value": [{ "name": "RETENTION_UNKNOWN", "number": 0 }, { "name": "RETENTION_RUNTIME", "number": 1 }, { "name": "RETENTION_SOURCE", "number": 2 }] }, { "name": "OptionTargetType", "value": [{ "name": "TARGET_TYPE_UNKNOWN", "number": 0 }, { "name": "TARGET_TYPE_FILE", "number": 1 }, { "name": "TARGET_TYPE_EXTENSION_RANGE", "number": 2 }, { "name": "TARGET_TYPE_MESSAGE", "number": 3 }, { "name": "TARGET_TYPE_FIELD", "number": 4 }, { "name": "TARGET_TYPE_ONEOF", "number": 5 }, { "name": "TARGET_TYPE_ENUM", "number": 6 }, { "name": "TARGET_TYPE_ENUM_ENTRY", "number": 7 }, { "name": "TARGET_TYPE_SERVICE", "number": 8 }, { "name": "TARGET_TYPE_METHOD", "number": 9 }] }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "OneofOptions", "field": [{ "name": "features", "number": 1, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "EnumOptions", "field": [{ "name": "allow_alias", "number": 2, "type": 8, "label": 1 }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated_legacy_json_field_conflicts", "number": 6, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "features", "number": 7, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "EnumValueOptions", "field": [{ "name": "deprecated", "number": 1, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "features", "number": 2, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "debug_redact", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "feature_support", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions.FeatureSupport" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "ServiceOptions", "field": [{ "name": "features", "number": 34, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "deprecated", "number": 33, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "MethodOptions", "field": [{ "name": "deprecated", "number": 33, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "idempotency_level", "number": 34, "type": 14, "label": 1, "typeName": ".google.protobuf.MethodOptions.IdempotencyLevel", "defaultValue": "IDEMPOTENCY_UNKNOWN" }, { "name": "features", "number": 35, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "enumType": [{ "name": "IdempotencyLevel", "value": [{ "name": "IDEMPOTENCY_UNKNOWN", "number": 0 }, { "name": "NO_SIDE_EFFECTS", "number": 1 }, { "name": "IDEMPOTENT", "number": 2 }] }], "extensionRange": [{ "start": 1e3, "end": 536870912 }] }, { "name": "UninterpretedOption", "field": [{ "name": "name", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption.NamePart" }, { "name": "identifier_value", "number": 3, "type": 9, "label": 1 }, { "name": "positive_int_value", "number": 4, "type": 4, "label": 1 }, { "name": "negative_int_value", "number": 5, "type": 3, "label": 1 }, { "name": "double_value", "number": 6, "type": 1, "label": 1 }, { "name": "string_value", "number": 7, "type": 12, "label": 1 }, { "name": "aggregate_value", "number": 8, "type": 9, "label": 1 }], "nestedType": [{ "name": "NamePart", "field": [{ "name": "name_part", "number": 1, "type": 9, "label": 2 }, { "name": "is_extension", "number": 2, "type": 8, "label": 2 }] }] }, { "name": "FeatureSet", "field": [{ "name": "field_presence", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.FieldPresence", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "EXPLICIT", "edition": 900 }, { "value": "IMPLICIT", "edition": 999 }, { "value": "EXPLICIT", "edition": 1e3 }] } }, { "name": "enum_type", "number": 2, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.EnumType", "options": { "retention": 1, "targets": [6, 1], "editionDefaults": [{ "value": "CLOSED", "edition": 900 }, { "value": "OPEN", "edition": 999 }] } }, { "name": "repeated_field_encoding", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.RepeatedFieldEncoding", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "EXPANDED", "edition": 900 }, { "value": "PACKED", "edition": 999 }] } }, { "name": "utf8_validation", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.Utf8Validation", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "NONE", "edition": 900 }, { "value": "VERIFY", "edition": 999 }] } }, { "name": "message_encoding", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.MessageEncoding", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "LENGTH_PREFIXED", "edition": 900 }] } }, { "name": "json_format", "number": 6, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.JsonFormat", "options": { "retention": 1, "targets": [3, 6, 1], "editionDefaults": [{ "value": "LEGACY_BEST_EFFORT", "edition": 900 }, { "value": "ALLOW", "edition": 999 }] } }, { "name": "enforce_naming_style", "number": 7, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.EnforceNamingStyle", "options": { "retention": 2, "targets": [1, 2, 3, 4, 5, 6, 7, 8, 9], "editionDefaults": [{ "value": "STYLE_LEGACY", "edition": 900 }, { "value": "STYLE2024", "edition": 1001 }] } }], "enumType": [{ "name": "FieldPresence", "value": [{ "name": "FIELD_PRESENCE_UNKNOWN", "number": 0 }, { "name": "EXPLICIT", "number": 1 }, { "name": "IMPLICIT", "number": 2 }, { "name": "LEGACY_REQUIRED", "number": 3 }] }, { "name": "EnumType", "value": [{ "name": "ENUM_TYPE_UNKNOWN", "number": 0 }, { "name": "OPEN", "number": 1 }, { "name": "CLOSED", "number": 2 }] }, { "name": "RepeatedFieldEncoding", "value": [{ "name": "REPEATED_FIELD_ENCODING_UNKNOWN", "number": 0 }, { "name": "PACKED", "number": 1 }, { "name": "EXPANDED", "number": 2 }] }, { "name": "Utf8Validation", "value": [{ "name": "UTF8_VALIDATION_UNKNOWN", "number": 0 }, { "name": "VERIFY", "number": 2 }, { "name": "NONE", "number": 3 }] }, { "name": "MessageEncoding", "value": [{ "name": "MESSAGE_ENCODING_UNKNOWN", "number": 0 }, { "name": "LENGTH_PREFIXED", "number": 1 }, { "name": "DELIMITED", "number": 2 }] }, { "name": "JsonFormat", "value": [{ "name": "JSON_FORMAT_UNKNOWN", "number": 0 }, { "name": "ALLOW", "number": 1 }, { "name": "LEGACY_BEST_EFFORT", "number": 2 }] }, { "name": "EnforceNamingStyle", "value": [{ "name": "ENFORCE_NAMING_STYLE_UNKNOWN", "number": 0 }, { "name": "STYLE2024", "number": 1 }, { "name": "STYLE_LEGACY", "number": 2 }] }], "extensionRange": [{ "start": 1e3, "end": 9995 }, { "start": 9995, "end": 1e4 }, { "start": 1e4, "end": 10001 }] }, { "name": "FeatureSetDefaults", "field": [{ "name": "defaults", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.FeatureSetDefaults.FeatureSetEditionDefault" }, { "name": "minimum_edition", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "maximum_edition", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }], "nestedType": [{ "name": "FeatureSetEditionDefault", "field": [{ "name": "edition", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "overridable_features", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "fixed_features", "number": 5, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }] }] }, { "name": "SourceCodeInfo", "field": [{ "name": "location", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.SourceCodeInfo.Location" }], "nestedType": [{ "name": "Location", "field": [{ "name": "path", "number": 1, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "span", "number": 2, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "leading_comments", "number": 3, "type": 9, "label": 1 }, { "name": "trailing_comments", "number": 4, "type": 9, "label": 1 }, { "name": "leading_detached_comments", "number": 6, "type": 9, "label": 3 }] }], "extensionRange": [{ "start": 536e6, "end": 536000001 }] }, { "name": "GeneratedCodeInfo", "field": [{ "name": "annotation", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.GeneratedCodeInfo.Annotation" }], "nestedType": [{ "name": "Annotation", "field": [{ "name": "path", "number": 1, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "source_file", "number": 2, "type": 9, "label": 1 }, { "name": "begin", "number": 3, "type": 5, "label": 1 }, { "name": "end", "number": 4, "type": 5, "label": 1 }, { "name": "semantic", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.GeneratedCodeInfo.Annotation.Semantic" }], "enumType": [{ "name": "Semantic", "value": [{ "name": "NONE", "number": 0 }, { "name": "SET", "number": 1 }, { "name": "ALIAS", "number": 2 }] }] }] }], "enumType": [{ "name": "Edition", "value": [{ "name": "EDITION_UNKNOWN", "number": 0 }, { "name": "EDITION_LEGACY", "number": 900 }, { "name": "EDITION_PROTO2", "number": 998 }, { "name": "EDITION_PROTO3", "number": 999 }, { "name": "EDITION_2023", "number": 1e3 }, { "name": "EDITION_2024", "number": 1001 }, { "name": "EDITION_1_TEST_ONLY", "number": 1 }, { "name": "EDITION_2_TEST_ONLY", "number": 2 }, { "name": "EDITION_99997_TEST_ONLY", "number": 99997 }, { "name": "EDITION_99998_TEST_ONLY", "number": 99998 }, { "name": "EDITION_99999_TEST_ONLY", "number": 99999 }, { "name": "EDITION_MAX", "number": 2147483647 }] }] });
  var FileDescriptorProtoSchema = /* @__PURE__ */ messageDesc(file_google_protobuf_descriptor, 1);
  var ExtensionRangeOptions_VerificationState;
  (function(ExtensionRangeOptions_VerificationState2) {
    ExtensionRangeOptions_VerificationState2[ExtensionRangeOptions_VerificationState2["DECLARATION"] = 0] = "DECLARATION";
    ExtensionRangeOptions_VerificationState2[ExtensionRangeOptions_VerificationState2["UNVERIFIED"] = 1] = "UNVERIFIED";
  })(ExtensionRangeOptions_VerificationState || (ExtensionRangeOptions_VerificationState = {}));
  var FieldDescriptorProto_Type;
  (function(FieldDescriptorProto_Type2) {
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["DOUBLE"] = 1] = "DOUBLE";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["FLOAT"] = 2] = "FLOAT";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["INT64"] = 3] = "INT64";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["UINT64"] = 4] = "UINT64";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["INT32"] = 5] = "INT32";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["FIXED64"] = 6] = "FIXED64";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["FIXED32"] = 7] = "FIXED32";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["BOOL"] = 8] = "BOOL";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["STRING"] = 9] = "STRING";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["GROUP"] = 10] = "GROUP";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["MESSAGE"] = 11] = "MESSAGE";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["BYTES"] = 12] = "BYTES";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["UINT32"] = 13] = "UINT32";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["ENUM"] = 14] = "ENUM";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["SFIXED32"] = 15] = "SFIXED32";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["SFIXED64"] = 16] = "SFIXED64";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["SINT32"] = 17] = "SINT32";
    FieldDescriptorProto_Type2[FieldDescriptorProto_Type2["SINT64"] = 18] = "SINT64";
  })(FieldDescriptorProto_Type || (FieldDescriptorProto_Type = {}));
  var FieldDescriptorProto_Label;
  (function(FieldDescriptorProto_Label2) {
    FieldDescriptorProto_Label2[FieldDescriptorProto_Label2["OPTIONAL"] = 1] = "OPTIONAL";
    FieldDescriptorProto_Label2[FieldDescriptorProto_Label2["REPEATED"] = 3] = "REPEATED";
    FieldDescriptorProto_Label2[FieldDescriptorProto_Label2["REQUIRED"] = 2] = "REQUIRED";
  })(FieldDescriptorProto_Label || (FieldDescriptorProto_Label = {}));
  var FileOptions_OptimizeMode;
  (function(FileOptions_OptimizeMode2) {
    FileOptions_OptimizeMode2[FileOptions_OptimizeMode2["SPEED"] = 1] = "SPEED";
    FileOptions_OptimizeMode2[FileOptions_OptimizeMode2["CODE_SIZE"] = 2] = "CODE_SIZE";
    FileOptions_OptimizeMode2[FileOptions_OptimizeMode2["LITE_RUNTIME"] = 3] = "LITE_RUNTIME";
  })(FileOptions_OptimizeMode || (FileOptions_OptimizeMode = {}));
  var FieldOptions_CType;
  (function(FieldOptions_CType2) {
    FieldOptions_CType2[FieldOptions_CType2["STRING"] = 0] = "STRING";
    FieldOptions_CType2[FieldOptions_CType2["CORD"] = 1] = "CORD";
    FieldOptions_CType2[FieldOptions_CType2["STRING_PIECE"] = 2] = "STRING_PIECE";
  })(FieldOptions_CType || (FieldOptions_CType = {}));
  var FieldOptions_JSType;
  (function(FieldOptions_JSType2) {
    FieldOptions_JSType2[FieldOptions_JSType2["JS_NORMAL"] = 0] = "JS_NORMAL";
    FieldOptions_JSType2[FieldOptions_JSType2["JS_STRING"] = 1] = "JS_STRING";
    FieldOptions_JSType2[FieldOptions_JSType2["JS_NUMBER"] = 2] = "JS_NUMBER";
  })(FieldOptions_JSType || (FieldOptions_JSType = {}));
  var FieldOptions_OptionRetention;
  (function(FieldOptions_OptionRetention2) {
    FieldOptions_OptionRetention2[FieldOptions_OptionRetention2["RETENTION_UNKNOWN"] = 0] = "RETENTION_UNKNOWN";
    FieldOptions_OptionRetention2[FieldOptions_OptionRetention2["RETENTION_RUNTIME"] = 1] = "RETENTION_RUNTIME";
    FieldOptions_OptionRetention2[FieldOptions_OptionRetention2["RETENTION_SOURCE"] = 2] = "RETENTION_SOURCE";
  })(FieldOptions_OptionRetention || (FieldOptions_OptionRetention = {}));
  var FieldOptions_OptionTargetType;
  (function(FieldOptions_OptionTargetType2) {
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_UNKNOWN"] = 0] = "TARGET_TYPE_UNKNOWN";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_FILE"] = 1] = "TARGET_TYPE_FILE";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_EXTENSION_RANGE"] = 2] = "TARGET_TYPE_EXTENSION_RANGE";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_MESSAGE"] = 3] = "TARGET_TYPE_MESSAGE";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_FIELD"] = 4] = "TARGET_TYPE_FIELD";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_ONEOF"] = 5] = "TARGET_TYPE_ONEOF";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_ENUM"] = 6] = "TARGET_TYPE_ENUM";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_ENUM_ENTRY"] = 7] = "TARGET_TYPE_ENUM_ENTRY";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_SERVICE"] = 8] = "TARGET_TYPE_SERVICE";
    FieldOptions_OptionTargetType2[FieldOptions_OptionTargetType2["TARGET_TYPE_METHOD"] = 9] = "TARGET_TYPE_METHOD";
  })(FieldOptions_OptionTargetType || (FieldOptions_OptionTargetType = {}));
  var MethodOptions_IdempotencyLevel;
  (function(MethodOptions_IdempotencyLevel2) {
    MethodOptions_IdempotencyLevel2[MethodOptions_IdempotencyLevel2["IDEMPOTENCY_UNKNOWN"] = 0] = "IDEMPOTENCY_UNKNOWN";
    MethodOptions_IdempotencyLevel2[MethodOptions_IdempotencyLevel2["NO_SIDE_EFFECTS"] = 1] = "NO_SIDE_EFFECTS";
    MethodOptions_IdempotencyLevel2[MethodOptions_IdempotencyLevel2["IDEMPOTENT"] = 2] = "IDEMPOTENT";
  })(MethodOptions_IdempotencyLevel || (MethodOptions_IdempotencyLevel = {}));
  var FeatureSet_FieldPresence;
  (function(FeatureSet_FieldPresence2) {
    FeatureSet_FieldPresence2[FeatureSet_FieldPresence2["FIELD_PRESENCE_UNKNOWN"] = 0] = "FIELD_PRESENCE_UNKNOWN";
    FeatureSet_FieldPresence2[FeatureSet_FieldPresence2["EXPLICIT"] = 1] = "EXPLICIT";
    FeatureSet_FieldPresence2[FeatureSet_FieldPresence2["IMPLICIT"] = 2] = "IMPLICIT";
    FeatureSet_FieldPresence2[FeatureSet_FieldPresence2["LEGACY_REQUIRED"] = 3] = "LEGACY_REQUIRED";
  })(FeatureSet_FieldPresence || (FeatureSet_FieldPresence = {}));
  var FeatureSet_EnumType;
  (function(FeatureSet_EnumType2) {
    FeatureSet_EnumType2[FeatureSet_EnumType2["ENUM_TYPE_UNKNOWN"] = 0] = "ENUM_TYPE_UNKNOWN";
    FeatureSet_EnumType2[FeatureSet_EnumType2["OPEN"] = 1] = "OPEN";
    FeatureSet_EnumType2[FeatureSet_EnumType2["CLOSED"] = 2] = "CLOSED";
  })(FeatureSet_EnumType || (FeatureSet_EnumType = {}));
  var FeatureSet_RepeatedFieldEncoding;
  (function(FeatureSet_RepeatedFieldEncoding2) {
    FeatureSet_RepeatedFieldEncoding2[FeatureSet_RepeatedFieldEncoding2["REPEATED_FIELD_ENCODING_UNKNOWN"] = 0] = "REPEATED_FIELD_ENCODING_UNKNOWN";
    FeatureSet_RepeatedFieldEncoding2[FeatureSet_RepeatedFieldEncoding2["PACKED"] = 1] = "PACKED";
    FeatureSet_RepeatedFieldEncoding2[FeatureSet_RepeatedFieldEncoding2["EXPANDED"] = 2] = "EXPANDED";
  })(FeatureSet_RepeatedFieldEncoding || (FeatureSet_RepeatedFieldEncoding = {}));
  var FeatureSet_Utf8Validation;
  (function(FeatureSet_Utf8Validation2) {
    FeatureSet_Utf8Validation2[FeatureSet_Utf8Validation2["UTF8_VALIDATION_UNKNOWN"] = 0] = "UTF8_VALIDATION_UNKNOWN";
    FeatureSet_Utf8Validation2[FeatureSet_Utf8Validation2["VERIFY"] = 2] = "VERIFY";
    FeatureSet_Utf8Validation2[FeatureSet_Utf8Validation2["NONE"] = 3] = "NONE";
  })(FeatureSet_Utf8Validation || (FeatureSet_Utf8Validation = {}));
  var FeatureSet_MessageEncoding;
  (function(FeatureSet_MessageEncoding2) {
    FeatureSet_MessageEncoding2[FeatureSet_MessageEncoding2["MESSAGE_ENCODING_UNKNOWN"] = 0] = "MESSAGE_ENCODING_UNKNOWN";
    FeatureSet_MessageEncoding2[FeatureSet_MessageEncoding2["LENGTH_PREFIXED"] = 1] = "LENGTH_PREFIXED";
    FeatureSet_MessageEncoding2[FeatureSet_MessageEncoding2["DELIMITED"] = 2] = "DELIMITED";
  })(FeatureSet_MessageEncoding || (FeatureSet_MessageEncoding = {}));
  var FeatureSet_JsonFormat;
  (function(FeatureSet_JsonFormat2) {
    FeatureSet_JsonFormat2[FeatureSet_JsonFormat2["JSON_FORMAT_UNKNOWN"] = 0] = "JSON_FORMAT_UNKNOWN";
    FeatureSet_JsonFormat2[FeatureSet_JsonFormat2["ALLOW"] = 1] = "ALLOW";
    FeatureSet_JsonFormat2[FeatureSet_JsonFormat2["LEGACY_BEST_EFFORT"] = 2] = "LEGACY_BEST_EFFORT";
  })(FeatureSet_JsonFormat || (FeatureSet_JsonFormat = {}));
  var FeatureSet_EnforceNamingStyle;
  (function(FeatureSet_EnforceNamingStyle2) {
    FeatureSet_EnforceNamingStyle2[FeatureSet_EnforceNamingStyle2["ENFORCE_NAMING_STYLE_UNKNOWN"] = 0] = "ENFORCE_NAMING_STYLE_UNKNOWN";
    FeatureSet_EnforceNamingStyle2[FeatureSet_EnforceNamingStyle2["STYLE2024"] = 1] = "STYLE2024";
    FeatureSet_EnforceNamingStyle2[FeatureSet_EnforceNamingStyle2["STYLE_LEGACY"] = 2] = "STYLE_LEGACY";
  })(FeatureSet_EnforceNamingStyle || (FeatureSet_EnforceNamingStyle = {}));
  var GeneratedCodeInfo_Annotation_Semantic;
  (function(GeneratedCodeInfo_Annotation_Semantic2) {
    GeneratedCodeInfo_Annotation_Semantic2[GeneratedCodeInfo_Annotation_Semantic2["NONE"] = 0] = "NONE";
    GeneratedCodeInfo_Annotation_Semantic2[GeneratedCodeInfo_Annotation_Semantic2["SET"] = 1] = "SET";
    GeneratedCodeInfo_Annotation_Semantic2[GeneratedCodeInfo_Annotation_Semantic2["ALIAS"] = 2] = "ALIAS";
  })(GeneratedCodeInfo_Annotation_Semantic || (GeneratedCodeInfo_Annotation_Semantic = {}));
  var Edition;
  (function(Edition2) {
    Edition2[Edition2["EDITION_UNKNOWN"] = 0] = "EDITION_UNKNOWN";
    Edition2[Edition2["EDITION_LEGACY"] = 900] = "EDITION_LEGACY";
    Edition2[Edition2["EDITION_PROTO2"] = 998] = "EDITION_PROTO2";
    Edition2[Edition2["EDITION_PROTO3"] = 999] = "EDITION_PROTO3";
    Edition2[Edition2["EDITION_2023"] = 1e3] = "EDITION_2023";
    Edition2[Edition2["EDITION_2024"] = 1001] = "EDITION_2024";
    Edition2[Edition2["EDITION_1_TEST_ONLY"] = 1] = "EDITION_1_TEST_ONLY";
    Edition2[Edition2["EDITION_2_TEST_ONLY"] = 2] = "EDITION_2_TEST_ONLY";
    Edition2[Edition2["EDITION_99997_TEST_ONLY"] = 99997] = "EDITION_99997_TEST_ONLY";
    Edition2[Edition2["EDITION_99998_TEST_ONLY"] = 99998] = "EDITION_99998_TEST_ONLY";
    Edition2[Edition2["EDITION_99999_TEST_ONLY"] = 99999] = "EDITION_99999_TEST_ONLY";
    Edition2[Edition2["EDITION_MAX"] = 2147483647] = "EDITION_MAX";
  })(Edition || (Edition = {}));

  // node_modules/@bufbuild/protobuf/dist/esm/from-binary.js
  var readDefaults = {
    readUnknownFields: true
  };
  function makeReadOptions(options) {
    return options ? Object.assign(Object.assign({}, readDefaults), options) : readDefaults;
  }
  function fromBinary(schema, bytes, options) {
    const msg = reflect(schema, void 0, false);
    readMessage(msg, new BinaryReader(bytes), makeReadOptions(options), false, bytes.byteLength);
    return msg.message;
  }
  function readMessage(message, reader, options, delimited, lengthOrDelimitedFieldNo) {
    var _a;
    const end = delimited ? reader.len : reader.pos + lengthOrDelimitedFieldNo;
    let fieldNo;
    let wireType;
    const unknownFields = (_a = message.getUnknown()) !== null && _a !== void 0 ? _a : [];
    while (reader.pos < end) {
      [fieldNo, wireType] = reader.tag();
      if (delimited && wireType == WireType.EndGroup) {
        break;
      }
      const field = message.findNumber(fieldNo);
      if (!field) {
        const data = reader.skip(wireType, fieldNo);
        if (options.readUnknownFields) {
          unknownFields.push({ no: fieldNo, wireType, data });
        }
        continue;
      }
      readField(message, reader, field, wireType, options);
    }
    if (delimited) {
      if (wireType != WireType.EndGroup || fieldNo !== lengthOrDelimitedFieldNo) {
        throw new Error("invalid end group tag");
      }
    }
    if (unknownFields.length > 0) {
      message.setUnknown(unknownFields);
    }
  }
  function readField(message, reader, field, wireType, options) {
    switch (field.fieldKind) {
      case "scalar":
        message.set(field, readScalar(reader, field.scalar));
        break;
      case "enum":
        message.set(field, readScalar(reader, ScalarType.INT32));
        break;
      case "message":
        message.set(field, readMessageField(reader, options, field, message.get(field)));
        break;
      case "list":
        readListField(reader, wireType, message.get(field), options);
        break;
      case "map":
        readMapEntry(reader, message.get(field), options);
        break;
    }
  }
  function readMapEntry(reader, map, options) {
    const field = map.field();
    let key;
    let val;
    const end = reader.pos + reader.uint32();
    while (reader.pos < end) {
      const [fieldNo] = reader.tag();
      switch (fieldNo) {
        case 1:
          key = readScalar(reader, field.mapKey);
          break;
        case 2:
          switch (field.mapKind) {
            case "scalar":
              val = readScalar(reader, field.scalar);
              break;
            case "enum":
              val = reader.int32();
              break;
            case "message":
              val = readMessageField(reader, options, field);
              break;
          }
          break;
      }
    }
    if (key === void 0) {
      key = scalarZeroValue(field.mapKey, false);
    }
    if (val === void 0) {
      switch (field.mapKind) {
        case "scalar":
          val = scalarZeroValue(field.scalar, false);
          break;
        case "enum":
          val = field.enum.values[0].number;
          break;
        case "message":
          val = reflect(field.message, void 0, false);
          break;
      }
    }
    map.set(key, val);
  }
  function readListField(reader, wireType, list, options) {
    var _a;
    const field = list.field();
    if (field.listKind === "message") {
      list.add(readMessageField(reader, options, field));
      return;
    }
    const scalarType = (_a = field.scalar) !== null && _a !== void 0 ? _a : ScalarType.INT32;
    const packed = wireType == WireType.LengthDelimited && scalarType != ScalarType.STRING && scalarType != ScalarType.BYTES;
    if (!packed) {
      list.add(readScalar(reader, scalarType));
      return;
    }
    const e = reader.uint32() + reader.pos;
    while (reader.pos < e) {
      list.add(readScalar(reader, scalarType));
    }
  }
  function readMessageField(reader, options, field, mergeMessage) {
    const delimited = field.delimitedEncoding;
    const message = mergeMessage !== null && mergeMessage !== void 0 ? mergeMessage : reflect(field.message, void 0, false);
    readMessage(message, reader, options, delimited, delimited ? field.number : reader.uint32());
    return message;
  }
  function readScalar(reader, type) {
    switch (type) {
      case ScalarType.STRING:
        return reader.string();
      case ScalarType.BOOL:
        return reader.bool();
      case ScalarType.DOUBLE:
        return reader.double();
      case ScalarType.FLOAT:
        return reader.float();
      case ScalarType.INT32:
        return reader.int32();
      case ScalarType.INT64:
        return reader.int64();
      case ScalarType.UINT64:
        return reader.uint64();
      case ScalarType.FIXED64:
        return reader.fixed64();
      case ScalarType.BYTES:
        return reader.bytes();
      case ScalarType.FIXED32:
        return reader.fixed32();
      case ScalarType.SFIXED32:
        return reader.sfixed32();
      case ScalarType.SFIXED64:
        return reader.sfixed64();
      case ScalarType.SINT64:
        return reader.sint64();
      case ScalarType.UINT32:
        return reader.uint32();
      case ScalarType.SINT32:
        return reader.sint32();
    }
  }

  // node_modules/@bufbuild/protobuf/dist/esm/codegenv1/file.js
  function fileDesc(b64, imports) {
    var _a;
    const root = fromBinary(FileDescriptorProtoSchema, base64Decode(b64));
    root.messageType.forEach(restoreJsonNames);
    root.dependency = (_a = imports === null || imports === void 0 ? void 0 : imports.map((f) => f.proto.name)) !== null && _a !== void 0 ? _a : [];
    const reg = createFileRegistry(root, (protoFileName) => imports === null || imports === void 0 ? void 0 : imports.find((f) => f.proto.name === protoFileName));
    return reg.getFile(root.name);
  }

  // node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/timestamp_pb.js
  var file_google_protobuf_timestamp = /* @__PURE__ */ fileDesc("Ch9nb29nbGUvcHJvdG9idWYvdGltZXN0YW1wLnByb3RvEg9nb29nbGUucHJvdG9idWYiKwoJVGltZXN0YW1wEg8KB3NlY29uZHMYASABKAMSDQoFbmFub3MYAiABKAVChQEKE2NvbS5nb29nbGUucHJvdG9idWZCDlRpbWVzdGFtcFByb3RvUAFaMmdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3RpbWVzdGFtcHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");

  // runtime/pb/sf/ethereum/type/v2/type_pb.ts
  var file_sf_ethereum_type_v2_type = /* @__PURE__ */ fileDesc("Ch5zZi9ldGhlcmV1bS90eXBlL3YyL3R5cGUucHJvdG8SE3NmLmV0aGVyZXVtLnR5cGUudjIimwQKBUJsb2NrEgwKBGhhc2gYAiABKAwSDgoGbnVtYmVyGAMgASgEEgwKBHNpemUYBCABKAQSMAoGaGVhZGVyGAUgASgLMiAuc2YuZXRoZXJldW0udHlwZS52Mi5CbG9ja0hlYWRlchIwCgZ1bmNsZXMYBiADKAsyIC5zZi5ldGhlcmV1bS50eXBlLnYyLkJsb2NrSGVhZGVyEkEKEnRyYW5zYWN0aW9uX3RyYWNlcxgKIAMoCzIlLnNmLmV0aGVyZXVtLnR5cGUudjIuVHJhbnNhY3Rpb25UcmFjZRI7Cg9iYWxhbmNlX2NoYW5nZXMYCyADKAsyIi5zZi5ldGhlcmV1bS50eXBlLnYyLkJhbGFuY2VDaGFuZ2USPAoMZGV0YWlsX2xldmVsGAwgASgOMiYuc2YuZXRoZXJldW0udHlwZS52Mi5CbG9jay5EZXRhaWxMZXZlbBI1Cgxjb2RlX2NoYW5nZXMYFCADKAsyHy5zZi5ldGhlcmV1bS50eXBlLnYyLkNvZGVDaGFuZ2USLwoMc3lzdGVtX2NhbGxzGBUgAygLMhkuc2YuZXRoZXJldW0udHlwZS52Mi5DYWxsEgsKA3ZlchgBIAEoBSI9CgtEZXRhaWxMZXZlbBIYChRERVRBSUxMRVZFTF9FWFRFTkRFRBAAEhQKEERFVEFJTExFVkVMX0JBU0UQAkoECCgQKUoECCkQKkoECCoQKyLXBQoLQmxvY2tIZWFkZXISEwoLcGFyZW50X2hhc2gYASABKAwSEgoKdW5jbGVfaGFzaBgCIAEoDBIQCghjb2luYmFzZRgDIAEoDBISCgpzdGF0ZV9yb290GAQgASgMEhkKEXRyYW5zYWN0aW9uc19yb290GAUgASgMEhQKDHJlY2VpcHRfcm9vdBgGIAEoDBISCgpsb2dzX2Jsb29tGAcgASgMEi8KCmRpZmZpY3VsdHkYCCABKAsyGy5zZi5ldGhlcmV1bS50eXBlLnYyLkJpZ0ludBI5ChB0b3RhbF9kaWZmaWN1bHR5GBEgASgLMhsuc2YuZXRoZXJldW0udHlwZS52Mi5CaWdJbnRCAhgBEg4KBm51bWJlchgJIAEoBBIRCglnYXNfbGltaXQYCiABKAQSEAoIZ2FzX3VzZWQYCyABKAQSLQoJdGltZXN0YW1wGAwgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBISCgpleHRyYV9kYXRhGA0gASgMEhAKCG1peF9oYXNoGA4gASgMEg0KBW5vbmNlGA8gASgEEgwKBGhhc2gYECABKAwSNQoQYmFzZV9mZWVfcGVyX2dhcxgSIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50EhgKEHdpdGhkcmF3YWxzX3Jvb3QYEyABKAwSPQoNdHhfZGVwZW5kZW5jeRgUIAEoCzImLnNmLmV0aGVyZXVtLnR5cGUudjIuVWludDY0TmVzdGVkQXJyYXkSGgoNYmxvYl9nYXNfdXNlZBgWIAEoBEgAiAEBEhwKD2V4Y2Vzc19ibG9iX2dhcxgXIAEoBEgBiAEBEhoKEnBhcmVudF9iZWFjb25fcm9vdBgYIAEoDBIVCg1yZXF1ZXN0c19oYXNoGBkgASgMQhAKDl9ibG9iX2dhc191c2VkQhIKEF9leGNlc3NfYmxvYl9nYXMiQgoRVWludDY0TmVzdGVkQXJyYXkSLQoDdmFsGAEgAygLMiAuc2YuZXRoZXJldW0udHlwZS52Mi5VaW50NjRBcnJheSIaCgtVaW50NjRBcnJheRILCgN2YWwYASADKAQiFwoGQmlnSW50Eg0KBWJ5dGVzGAEgASgMIrgKChBUcmFuc2FjdGlvblRyYWNlEgoKAnRvGAEgASgMEg0KBW5vbmNlGAIgASgEEi4KCWdhc19wcmljZRgDIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50EhEKCWdhc19saW1pdBgEIAEoBBIqCgV2YWx1ZRgFIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50Eg0KBWlucHV0GAYgASgMEgkKAXYYByABKAwSCQoBchgIIAEoDBIJCgFzGAkgASgMEhAKCGdhc191c2VkGAogASgEEjgKBHR5cGUYDCABKA4yKi5zZi5ldGhlcmV1bS50eXBlLnYyLlRyYW5zYWN0aW9uVHJhY2UuVHlwZRI1CgthY2Nlc3NfbGlzdBgOIAMoCzIgLnNmLmV0aGVyZXVtLnR5cGUudjIuQWNjZXNzVHVwbGUSNAoPbWF4X2ZlZV9wZXJfZ2FzGAsgASgLMhsuc2YuZXRoZXJldW0udHlwZS52Mi5CaWdJbnQSPQoYbWF4X3ByaW9yaXR5X2ZlZV9wZXJfZ2FzGA0gASgLMhsuc2YuZXRoZXJldW0udHlwZS52Mi5CaWdJbnQSDQoFaW5kZXgYFCABKA0SDAoEaGFzaBgVIAEoDBIMCgRmcm9tGBYgASgMEhMKC3JldHVybl9kYXRhGBcgASgMEhIKCnB1YmxpY19rZXkYGCABKAwSFQoNYmVnaW5fb3JkaW5hbBgZIAEoBBITCgtlbmRfb3JkaW5hbBgaIAEoBBI7CgZzdGF0dXMYHiABKA4yKy5zZi5ldGhlcmV1bS50eXBlLnYyLlRyYW5zYWN0aW9uVHJhY2VTdGF0dXMSOAoHcmVjZWlwdBgfIAEoCzInLnNmLmV0aGVyZXVtLnR5cGUudjIuVHJhbnNhY3Rpb25SZWNlaXB0EigKBWNhbGxzGCAgAygLMhkuc2YuZXRoZXJldW0udHlwZS52Mi5DYWxsEhUKCGJsb2JfZ2FzGCEgASgESACIAQESOgoQYmxvYl9nYXNfZmVlX2NhcBgiIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50SAGIAQESEwoLYmxvYl9oYXNoZXMYIyADKAwSSgoXc2V0X2NvZGVfYXV0aG9yaXphdGlvbnMYJCADKAsyKS5zZi5ldGhlcmV1bS50eXBlLnYyLlNldENvZGVBdXRob3JpemF0aW9uIvoCCgRUeXBlEhMKD1RSWF9UWVBFX0xFR0FDWRAAEhgKFFRSWF9UWVBFX0FDQ0VTU19MSVNUEAESGAoUVFJYX1RZUEVfRFlOQU1JQ19GRUUQAhIRCg1UUlhfVFlQRV9CTE9CEAMSFQoRVFJYX1RZUEVfU0VUX0NPREUQBBIdChlUUlhfVFlQRV9BUkJJVFJVTV9ERVBPU0lUEGQSHgoaVFJYX1RZUEVfQVJCSVRSVU1fVU5TSUdORUQQZRIeChpUUlhfVFlQRV9BUkJJVFJVTV9DT05UUkFDVBBmEhsKF1RSWF9UWVBFX0FSQklUUlVNX1JFVFJZEGgSJgoiVFJYX1RZUEVfQVJCSVRSVU1fU1VCTUlUX1JFVFJZQUJMRRBpEh4KGlRSWF9UWVBFX0FSQklUUlVNX0lOVEVSTkFMEGoSHAoYVFJYX1RZUEVfQVJCSVRSVU1fTEVHQUNZEHgSHQoZVFJYX1RZUEVfT1BUSU1JU01fREVQT1NJVBB+QgsKCV9ibG9iX2dhc0ITChFfYmxvYl9nYXNfZmVlX2NhcCI0CgtBY2Nlc3NUdXBsZRIPCgdhZGRyZXNzGAEgASgMEhQKDHN0b3JhZ2Vfa2V5cxgCIAMoDCKiAQoUU2V0Q29kZUF1dGhvcml6YXRpb24SEQoJZGlzY2FyZGVkGAEgASgIEhAKCGNoYWluX2lkGAIgASgMEg8KB2FkZHJlc3MYCCABKAwSDQoFbm9uY2UYAyABKAQSCQoBdhgEIAEoDRIJCgFyGAUgASgMEgkKAXMYBiABKAwSFgoJYXV0aG9yaXR5GAcgASgMSACIAQFCDAoKX2F1dGhvcml0eSL8AQoSVHJhbnNhY3Rpb25SZWNlaXB0EhIKCnN0YXRlX3Jvb3QYASABKAwSGwoTY3VtdWxhdGl2ZV9nYXNfdXNlZBgCIAEoBBISCgpsb2dzX2Jsb29tGAMgASgMEiYKBGxvZ3MYBCADKAsyGC5zZi5ldGhlcmV1bS50eXBlLnYyLkxvZxIaCg1ibG9iX2dhc191c2VkGAUgASgESACIAQESOAoOYmxvYl9nYXNfcHJpY2UYBiABKAsyGy5zZi5ldGhlcmV1bS50eXBlLnYyLkJpZ0ludEgBiAEBQhAKDl9ibG9iX2dhc191c2VkQhEKD19ibG9iX2dhc19wcmljZSJoCgNMb2cSDwoHYWRkcmVzcxgBIAEoDBIOCgZ0b3BpY3MYAiADKAwSDAoEZGF0YRgDIAEoDBINCgVpbmRleBgEIAEoDRISCgpibG9ja0luZGV4GAYgASgNEg8KB29yZGluYWwYByABKAQioggKBENhbGwSDQoFaW5kZXgYASABKA0SFAoMcGFyZW50X2luZGV4GAIgASgNEg0KBWRlcHRoGAMgASgNEjAKCWNhbGxfdHlwZRgEIAEoDjIdLnNmLmV0aGVyZXVtLnR5cGUudjIuQ2FsbFR5cGUSDgoGY2FsbGVyGAUgASgMEg8KB2FkZHJlc3MYBiABKAwSIQoUYWRkcmVzc19kZWxlZ2F0ZXNfdG8YIiABKAxIAIgBARIqCgV2YWx1ZRgHIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50EhEKCWdhc19saW1pdBgIIAEoBBIUCgxnYXNfY29uc3VtZWQYCSABKAQSEwoLcmV0dXJuX2RhdGEYDSABKAwSDQoFaW5wdXQYDiABKAwSFQoNZXhlY3V0ZWRfY29kZRgPIAEoCBIPCgdzdWljaWRlGBAgASgIEkgKEGtlY2Nha19wcmVpbWFnZXMYFCADKAsyLi5zZi5ldGhlcmV1bS50eXBlLnYyLkNhbGwuS2VjY2FrUHJlaW1hZ2VzRW50cnkSOwoPc3RvcmFnZV9jaGFuZ2VzGBUgAygLMiIuc2YuZXRoZXJldW0udHlwZS52Mi5TdG9yYWdlQ2hhbmdlEjsKD2JhbGFuY2VfY2hhbmdlcxgWIAMoCzIiLnNmLmV0aGVyZXVtLnR5cGUudjIuQmFsYW5jZUNoYW5nZRI3Cg1ub25jZV9jaGFuZ2VzGBggAygLMiAuc2YuZXRoZXJldW0udHlwZS52Mi5Ob25jZUNoYW5nZRImCgRsb2dzGBkgAygLMhguc2YuZXRoZXJldW0udHlwZS52Mi5Mb2cSNQoMY29kZV9jaGFuZ2VzGBogAygLMh8uc2YuZXRoZXJldW0udHlwZS52Mi5Db2RlQ2hhbmdlEjMKC2dhc19jaGFuZ2VzGBwgAygLMh4uc2YuZXRoZXJldW0udHlwZS52Mi5HYXNDaGFuZ2USFQoNc3RhdHVzX2ZhaWxlZBgKIAEoCBIXCg9zdGF0dXNfcmV2ZXJ0ZWQYDCABKAgSFgoOZmFpbHVyZV9yZWFzb24YCyABKAkSFgoOc3RhdGVfcmV2ZXJ0ZWQYHiABKAgSFQoNYmVnaW5fb3JkaW5hbBgfIAEoBBITCgtlbmRfb3JkaW5hbBggIAEoBBJDChFhY2NvdW50X2NyZWF0aW9ucxghIAMoCzIkLnNmLmV0aGVyZXVtLnR5cGUudjIuQWNjb3VudENyZWF0aW9uQgIYARo2ChRLZWNjYWtQcmVpbWFnZXNFbnRyeRILCgNrZXkYASABKAkSDQoFdmFsdWUYAiABKAk6AjgBQhcKFV9hZGRyZXNzX2RlbGVnYXRlc190b0oECBsQHEoECB0QHkoECDIQM0oECDMQNEoECDwQPSJkCg1TdG9yYWdlQ2hhbmdlEg8KB2FkZHJlc3MYASABKAwSCwoDa2V5GAIgASgMEhEKCW9sZF92YWx1ZRgDIAEoDBIRCgluZXdfdmFsdWUYBCABKAwSDwoHb3JkaW5hbBgFIAEoBCLnBQoNQmFsYW5jZUNoYW5nZRIPCgdhZGRyZXNzGAEgASgMEi4KCW9sZF92YWx1ZRgCIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50Ei4KCW5ld192YWx1ZRgDIAEoCzIbLnNmLmV0aGVyZXVtLnR5cGUudjIuQmlnSW50EjkKBnJlYXNvbhgEIAEoDjIpLnNmLmV0aGVyZXVtLnR5cGUudjIuQmFsYW5jZUNoYW5nZS5SZWFzb24SDwoHb3JkaW5hbBgFIAEoBCKYBAoGUmVhc29uEhIKDlJFQVNPTl9VTktOT1dOEAASHAoYUkVBU09OX1JFV0FSRF9NSU5FX1VOQ0xFEAESHAoYUkVBU09OX1JFV0FSRF9NSU5FX0JMT0NLEAISHgoaUkVBU09OX0RBT19SRUZVTkRfQ09OVFJBQ1QQAxIdChlSRUFTT05fREFPX0FESlVTVF9CQUxBTkNFEAQSEwoPUkVBU09OX1RSQU5TRkVSEAUSGgoWUkVBU09OX0dFTkVTSVNfQkFMQU5DRRAGEhIKDlJFQVNPTl9HQVNfQlVZEAcSIQodUkVBU09OX1JFV0FSRF9UUkFOU0FDVElPTl9GRUUQCBIbChdSRUFTT05fUkVXQVJEX0ZFRV9SRVNFVBAOEhUKEVJFQVNPTl9HQVNfUkVGVU5EEAkSGAoUUkVBU09OX1RPVUNIX0FDQ09VTlQQChIZChVSRUFTT05fU1VJQ0lERV9SRUZVTkQQCxIbChdSRUFTT05fU1VJQ0lERV9XSVRIRFJBVxANEiAKHFJFQVNPTl9DQUxMX0JBTEFOQ0VfT1ZFUlJJREUQDBIPCgtSRUFTT05fQlVSThAPEhUKEVJFQVNPTl9XSVRIRFJBV0FMEBASGgoWUkVBU09OX1JFV0FSRF9CTE9CX0ZFRRAREhgKFFJFQVNPTl9JTkNSRUFTRV9NSU5UEBISEQoNUkVBU09OX1JFVkVSVBATIlUKC05vbmNlQ2hhbmdlEg8KB2FkZHJlc3MYASABKAwSEQoJb2xkX3ZhbHVlGAIgASgEEhEKCW5ld192YWx1ZRgDIAEoBBIPCgdvcmRpbmFsGAQgASgEIjMKD0FjY291bnRDcmVhdGlvbhIPCgdhY2NvdW50GAEgASgMEg8KB29yZGluYWwYAiABKAQidgoKQ29kZUNoYW5nZRIPCgdhZGRyZXNzGAEgASgMEhAKCG9sZF9oYXNoGAIgASgMEhAKCG9sZF9jb2RlGAMgASgMEhAKCG5ld19oYXNoGAQgASgMEhAKCG5ld19jb2RlGAUgASgMEg8KB29yZGluYWwYBiABKAQi6QcKCUdhc0NoYW5nZRIRCglvbGRfdmFsdWUYASABKAQSEQoJbmV3X3ZhbHVlGAIgASgEEjUKBnJlYXNvbhgDIAEoDjIlLnNmLmV0aGVyZXVtLnR5cGUudjIuR2FzQ2hhbmdlLlJlYXNvbhIPCgdvcmRpbmFsGAQgASgEIu0GCgZSZWFzb24SEgoOUkVBU09OX1VOS05PV04QABIPCgtSRUFTT05fQ0FMTBABEhQKEFJFQVNPTl9DQUxMX0NPREUQAhIZChVSRUFTT05fQ0FMTF9EQVRBX0NPUFkQAxIUChBSRUFTT05fQ09ERV9DT1BZEAQSFwoTUkVBU09OX0NPREVfU1RPUkFHRRAFEhwKGFJFQVNPTl9DT05UUkFDVF9DUkVBVElPThAGEh0KGVJFQVNPTl9DT05UUkFDVF9DUkVBVElPTjIQBxIYChRSRUFTT05fREVMRUdBVEVfQ0FMTBAIEhQKEFJFQVNPTl9FVkVOVF9MT0cQCRIYChRSRUFTT05fRVhUX0NPREVfQ09QWRAKEhsKF1JFQVNPTl9GQUlMRURfRVhFQ1VUSU9OEAsSGAoUUkVBU09OX0lOVFJJTlNJQ19HQVMQDBIfChtSRUFTT05fUFJFQ09NUElMRURfQ09OVFJBQ1QQDRIhCh1SRUFTT05fUkVGVU5EX0FGVEVSX0VYRUNVVElPThAOEhEKDVJFQVNPTl9SRVRVUk4QDxIbChdSRUFTT05fUkVUVVJOX0RBVEFfQ09QWRAQEhEKDVJFQVNPTl9SRVZFUlQQERIYChRSRUFTT05fU0VMRl9ERVNUUlVDVBASEhYKElJFQVNPTl9TVEFUSUNfQ0FMTBATEhwKGFJFQVNPTl9TVEFURV9DT0xEX0FDQ0VTUxAUEh0KGVJFQVNPTl9UWF9JTklUSUFMX0JBTEFOQ0UQFRIVChFSRUFTT05fVFhfUkVGVU5EUxAWEiAKHFJFQVNPTl9UWF9MRUZUX09WRVJfUkVUVVJORUQQFxIfChtSRUFTT05fQ0FMTF9JTklUSUFMX0JBTEFOQ0UQGBIiCh5SRUFTT05fQ0FMTF9MRUZUX09WRVJfUkVUVVJORUQQGRIgChxSRUFTT05fV0lUTkVTU19DT05UUkFDVF9JTklUEBoSJAogUkVBU09OX1dJVE5FU1NfQ09OVFJBQ1RfQ1JFQVRJT04QGxIdChlSRUFTT05fV0lUTkVTU19DT0RFX0NIVU5LEBwSKwonUkVBU09OX1dJVE5FU1NfQ09OVFJBQ1RfQ09MTElTSU9OX0NIRUNLEB0SGAoUUkVBU09OX1RYX0RBVEFfRkxPT1IQHiJDCg9IZWFkZXJPbmx5QmxvY2sSMAoGaGVhZGVyGAUgASgLMiAuc2YuZXRoZXJldW0udHlwZS52Mi5CbG9ja0hlYWRlciKiAQoNQmxvY2tXaXRoUmVmcxIKCgJpZBgBIAEoCRIpCgVibG9jaxgCIAEoCzIaLnNmLmV0aGVyZXVtLnR5cGUudjIuQmxvY2sSRAoWdHJhbnNhY3Rpb25fdHJhY2VfcmVmcxgDIAEoCzIkLnNmLmV0aGVyZXVtLnR5cGUudjIuVHJhbnNhY3Rpb25SZWZzEhQKDGlycmV2ZXJzaWJsZRgEIAEoCCKGAQocVHJhbnNhY3Rpb25UcmFjZVdpdGhCbG9ja1JlZhI0CgV0cmFjZRgBIAEoCzIlLnNmLmV0aGVyZXVtLnR5cGUudjIuVHJhbnNhY3Rpb25UcmFjZRIwCglibG9ja19yZWYYAiABKAsyHS5zZi5ldGhlcmV1bS50eXBlLnYyLkJsb2NrUmVmIiEKD1RyYW5zYWN0aW9uUmVmcxIOCgZoYXNoZXMYASADKAwiKAoIQmxvY2tSZWYSDAoEaGFzaBgBIAEoDBIOCgZudW1iZXIYAiABKAQqTgoWVHJhbnNhY3Rpb25UcmFjZVN0YXR1cxILCgdVTktOT1dOEAASDQoJU1VDQ0VFREVEEAESCgoGRkFJTEVEEAISDAoIUkVWRVJURUQQAypZCghDYWxsVHlwZRIPCgtVTlNQRUNJRklFRBAAEggKBENBTEwQARIMCghDQUxMQ09ERRACEgwKCERFTEVHQVRFEAMSCgoGU1RBVElDEAQSCgoGQ1JFQVRFEAVCT1pNZ2l0aHViLmNvbS9zdHJlYW1pbmdmYXN0L2ZpcmVob3NlLWV0aGVyZXVtL3R5cGVzL3BiL3NmL2V0aGVyZXVtL3R5cGUvdjI7cGJldGhiBnByb3RvMw", [file_google_protobuf_timestamp]);
  var BlockSchema = /* @__PURE__ */ messageDesc(file_sf_ethereum_type_v2_type, 0);

  // runtime/pb/sf/substreams/sink/database/v1/database_pb.ts
  var file_sf_substreams_sink_database_v1_database = /* @__PURE__ */ fileDesc("Ci1zZi9zdWJzdHJlYW1zL3NpbmsvZGF0YWJhc2UvdjEvZGF0YWJhc2UucHJvdG8SHnNmLnN1YnN0cmVhbXMuc2luay5kYXRhYmFzZS52MSJVCg9EYXRhYmFzZUNoYW5nZXMSQgoNdGFibGVfY2hhbmdlcxgBIAMoCzIrLnNmLnN1YnN0cmVhbXMuc2luay5kYXRhYmFzZS52MS5UYWJsZUNoYW5nZSKCAwoLVGFibGVDaGFuZ2USDQoFdGFibGUYASABKAkSDAoCcGsYAiABKAlIABJLCgxjb21wb3NpdGVfcGsYBiABKAsyMy5zZi5zdWJzdHJlYW1zLnNpbmsuZGF0YWJhc2UudjEuQ29tcG9zaXRlUHJpbWFyeUtleUgAEg8KB29yZGluYWwYAyABKAQSSAoJb3BlcmF0aW9uGAQgASgOMjUuc2Yuc3Vic3RyZWFtcy5zaW5rLmRhdGFiYXNlLnYxLlRhYmxlQ2hhbmdlLk9wZXJhdGlvbhI1CgZmaWVsZHMYBSADKAsyJS5zZi5zdWJzdHJlYW1zLnNpbmsuZGF0YWJhc2UudjEuRmllbGQiaAoJT3BlcmF0aW9uEhkKFU9QRVJBVElPTl9VTlNQRUNJRklFRBAAEhQKEE9QRVJBVElPTl9DUkVBVEUQARIUChBPUEVSQVRJT05fVVBEQVRFEAISFAoQT1BFUkFUSU9OX0RFTEVURRADQg0KC3ByaW1hcnlfa2V5Io8BChNDb21wb3NpdGVQcmltYXJ5S2V5EksKBGtleXMYASADKAsyPS5zZi5zdWJzdHJlYW1zLnNpbmsuZGF0YWJhc2UudjEuQ29tcG9zaXRlUHJpbWFyeUtleS5LZXlzRW50cnkaKwoJS2V5c0VudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoCToCOAEiOwoFRmllbGQSDAoEbmFtZRgBIAEoCRIRCgluZXdfdmFsdWUYAiABKAkSEQoJb2xkX3ZhbHVlGAMgASgJQmhaZmdpdGh1Yi5jb20vc3RyZWFtaW5nZmFzdC9zdWJzdHJlYW1zLXNpbmstZGF0YWJhc2UtY2hhbmdlcy9wYi9zZi9zdWJzdHJlYW1zL3NpbmsvZGF0YWJhc2UvdjE7cGJkYXRhYmFzZWIGcHJvdG8z");
  var DatabaseChangesSchema = /* @__PURE__ */ messageDesc(file_sf_substreams_sink_database_v1_database, 0);
  var TableChangeSchema = /* @__PURE__ */ messageDesc(file_sf_substreams_sink_database_v1_database, 1);
  var FieldSchema = /* @__PURE__ */ messageDesc(file_sf_substreams_sink_database_v1_database, 3);

  // runtime/prelude.ts
  var rocketAddress = bytesFromHex("0xae78736Cd615f374D3085123A210448E74Fc6393");
  var approvalTopic = bytesFromHex("0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925");
  var transferTopic = bytesFromHex("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef");
  function main() {
    const out = map_block(readInput());
    writeOutput(out);
  }
  function readInput() {
    return globalThis.input;
  }
  function writeOutput(output) {
    globalThis.output.set(output);
  }
  function map_block(data) {
    var _a, _b, _c, _d, _e;
    const block = fromBinary(BlockSchema, data);
    const changes = create(DatabaseChangesSchema);
    const blockNumberStr = (_b = (_a = block.header) == null ? void 0 : _a.number.toString()) != null ? _b : "";
    const blockTimestampStr = (_e = (_d = (_c = block.header) == null ? void 0 : _c.timestamp) == null ? void 0 : _d.seconds.toString()) != null ? _e : "";
    let trxCount = 0;
    let transferCount = 0;
    let approvalCount = 0;
    block.transactionTraces.forEach((trace) => {
      trxCount++;
      if (trace.status !== 1 /* SUCCEEDED */) {
        return;
      }
      trace.calls.forEach((call) => {
        if (call.stateReverted) {
          return;
        }
        call.logs.forEach((log) => {
          if (!bytesEqual(log.address, rocketAddress) || log.topics.length === 0) {
            return;
          }
          if (bytesEqual(log.topics[0], approvalTopic)) {
            approvalCount++;
            const change = create(TableChangeSchema);
            change.table = "Approval";
            change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` };
            change.operation = 1 /* CREATE */;
            change.ordinal = (0, import_bigInt.default)(0);
            change.fields = [
              create(FieldSchema, { name: "timestamp", newValue: blockTimestampStr }),
              create(FieldSchema, { name: "block_number", newValue: blockNumberStr }),
              create(FieldSchema, { name: "log_index", newValue: log.index.toString() }),
              create(FieldSchema, { name: "tx_hash", newValue: bytesToHex(trace.hash) }),
              create(FieldSchema, { name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
              create(FieldSchema, { name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
              create(FieldSchema, { name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) })
            ];
            changes.tableChanges.push(change);
            return;
          }
          if (bytesEqual(log.topics[0], transferTopic)) {
            transferCount++;
            const change = create(TableChangeSchema);
            change.table = "Transfer";
            change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` };
            change.operation = 1 /* CREATE */;
            change.ordinal = (0, import_bigInt.default)(0);
            change.fields = [
              create(FieldSchema, { name: "timestamp", newValue: blockTimestampStr }),
              create(FieldSchema, { name: "block_number", newValue: blockNumberStr }),
              create(FieldSchema, { name: "log_index", newValue: log.index.toString() }),
              create(FieldSchema, { name: "tx_hash", newValue: bytesToHex(trace.hash) }),
              create(FieldSchema, { name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
              create(FieldSchema, { name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
              create(FieldSchema, { name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) })
            ];
            changes.tableChanges.push(change);
            return;
          }
        });
      });
    });
    return {
      trxCount,
      transferCount,
      approvalCount
    };
  }
  function stripZeroBytes(input) {
    for (let i = 0; i !== input.length; i++) {
      if (input[i] !== 0) {
        return input.slice(i);
      }
    }
    return input;
  }
  var alphaCharCode = "a".charCodeAt(0) - 10;
  var digitCharCode = "0".charCodeAt(0);
  function bytesToHex(byteArray) {
    const chars = new Uint8Array(byteArray.length * 2);
    let p = 0;
    for (let i = 0; i < byteArray.length; i++) {
      let nibble = byteArray[i] >>> 4;
      chars[p++] = nibble > 9 ? nibble + alphaCharCode : nibble + digitCharCode;
      nibble = byteArray[i] & 15;
      chars[p++] = nibble > 9 ? nibble + alphaCharCode : nibble + digitCharCode;
    }
    return String.fromCharCode(...chars);
  }
  function bytesFromHex(hex) {
    if (hex.match(/^0(x|X)/)) hex = hex.slice(2);
    if (hex.length % 2 !== 0) hex = "0" + hex;
    const bytes = new Uint8Array(hex.length / 2);
    for (let i = 0, c = 0; c < hex.length; c += 2, i++) {
      bytes[i] = parseInt(hex.slice(c, c + 2), 16);
    }
    return bytes;
  }
  function bytesEqual(left, right) {
    if (left.length !== right.length) return false;
    for (let i = 0; i < left.byteLength; i++) {
      if (left[i] !== right[i]) return false;
    }
    return true;
  }
})();
