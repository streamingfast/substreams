var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
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

// shims/bigInt/index.js
var require_bigInt = __commonJS({
  "shims/bigInt/index.js"(exports, module) {
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
        if (r2 < 4 && A(t2, o) < 0)
          switch (r2) {
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
        for (var e2 = t2.length; 0 === t2[--e2]; )
          ;
        t2.length = e2 + 1;
      }
      function y(t2) {
        for (var e2 = new Array(t2), r2 = -1; ++r2 < t2; )
          e2[r2] = 0;
        return e2;
      }
      function g(t2) {
        return t2 > 0 ? Math.floor(t2) : Math.ceil(t2);
      }
      function c(t2, r2) {
        var o2, n2, i2 = t2.length, u2 = r2.length, p2 = new Array(i2), a2 = 0, s2 = e;
        for (n2 = 0; n2 < u2; n2++)
          a2 = (o2 = t2[n2] + r2[n2] + a2) >= s2 ? 1 : 0, p2[n2] = o2 - a2 * s2;
        for (; n2 < i2; )
          a2 = (o2 = t2[n2] + a2) === s2 ? 1 : 0, p2[n2++] = o2 - a2 * s2;
        return a2 > 0 && p2.push(a2), p2;
      }
      function m(t2, e2) {
        return t2.length >= e2.length ? c(t2, e2) : c(e2, t2);
      }
      function d(t2, r2) {
        var o2, n2, i2 = t2.length, u2 = new Array(i2), p2 = e;
        for (n2 = 0; n2 < i2; n2++)
          o2 = t2[n2] - p2 + r2, r2 = Math.floor(o2 / p2), u2[n2] = o2 - r2 * p2, r2 += 1;
        for (; r2 > 0; )
          u2[n2++] = r2 % p2, r2 = Math.floor(r2 / p2);
        return u2;
      }
      function b(t2, r2) {
        var o2, n2, i2 = t2.length, u2 = r2.length, p2 = new Array(i2), a2 = 0, s2 = e;
        for (o2 = 0; o2 < u2; o2++)
          (n2 = t2[o2] - a2 - r2[o2]) < 0 ? (n2 += s2, a2 = 1) : a2 = 0, p2[o2] = n2;
        for (o2 = u2; o2 < i2; o2++) {
          if (!((n2 = t2[o2] - a2) < 0)) {
            p2[o2++] = n2;
            break;
          }
          n2 += s2, p2[o2] = n2;
        }
        for (; o2 < i2; o2++)
          p2[o2] = t2[o2];
        return h(p2), p2;
      }
      function w(t2, r2, o2) {
        var n2, i2, u2 = t2.length, s2 = new Array(u2), l2 = -r2, f2 = e;
        for (n2 = 0; n2 < u2; n2++)
          i2 = t2[n2] + l2, l2 = Math.floor(i2 / f2), i2 %= f2, s2[n2] = i2 < 0 ? i2 + f2 : i2;
        return "number" == typeof (s2 = v(s2)) ? (o2 && (s2 = -s2), new a(s2)) : new p(s2, o2);
      }
      function S(t2, r2) {
        var o2, n2, i2, u2, p2 = t2.length, a2 = r2.length, s2 = y(p2 + a2), l2 = e;
        for (i2 = 0; i2 < p2; ++i2) {
          u2 = t2[i2];
          for (var f2 = 0; f2 < a2; ++f2)
            o2 = u2 * r2[f2] + s2[i2 + f2], n2 = Math.floor(o2 / l2), s2[i2 + f2] = o2 - n2 * l2, s2[i2 + f2 + 1] += n2;
        }
        return h(s2), s2;
      }
      function I(t2, r2) {
        var o2, n2, i2 = t2.length, u2 = new Array(i2), p2 = e, a2 = 0;
        for (n2 = 0; n2 < i2; n2++)
          o2 = t2[n2] * r2 + a2, a2 = Math.floor(o2 / p2), u2[n2] = o2 - a2 * p2;
        for (; a2 > 0; )
          u2[n2++] = a2 % p2, a2 = Math.floor(a2 / p2);
        return u2;
      }
      function q(t2, e2) {
        for (var r2 = []; e2-- > 0; )
          r2.push(0);
        return r2.concat(t2);
      }
      function M(t2, e2) {
        var r2 = Math.max(t2.length, e2.length);
        if (r2 <= 30)
          return S(t2, e2);
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
          for (var s2 = n2; s2 < u2; s2++)
            r2 = i2 * t2[s2] * 2 + p2[n2 + s2] + o2, o2 = Math.floor(r2 / a2), p2[n2 + s2] = r2 - o2 * a2;
          p2[n2 + u2] = o2;
        }
        return h(p2), p2;
      }
      function O(t2, e2) {
        var r2, o2, n2, i2, u2 = t2.length, p2 = y(u2);
        for (n2 = 0, r2 = u2 - 1; r2 >= 0; --r2)
          n2 = (i2 = 1e7 * n2 + t2[r2]) - (o2 = g(i2 / e2)) * e2, p2[r2] = 0 | o2;
        return [p2, 0 | n2];
      }
      function B(t2, r2) {
        var o2, n2 = K(r2);
        if (i)
          return [new s(t2.value / n2.value), new s(t2.value % n2.value)];
        var l2, c2 = t2.value, m2 = n2.value;
        if (0 === m2)
          throw new Error("Cannot divide by zero");
        if (t2.isSmall)
          return n2.isSmall ? [new a(g(c2 / m2)), new a(c2 % m2)] : [u[0], t2];
        if (n2.isSmall) {
          if (1 === m2)
            return [t2, u[0]];
          if (-1 == m2)
            return [t2.negate(), u[0]];
          var d2 = Math.abs(m2);
          if (d2 < e) {
            l2 = v((o2 = O(c2, d2))[0]);
            var w2 = o2[1];
            return t2.sign && (w2 = -w2), "number" == typeof l2 ? (t2.sign !== n2.sign && (l2 = -l2), [new a(l2), new a(w2)]) : [new p(l2, t2.sign !== n2.sign), new a(w2)];
          }
          m2 = f(d2);
        }
        var S2 = A(c2, m2);
        if (-1 === S2)
          return [u[0], t2];
        if (0 === S2)
          return [u[t2.sign === n2.sign ? 1 : -1], u[0]];
        o2 = c2.length + m2.length <= 200 ? function(t3, r3) {
          var o3, n3, i2, u2, p2, a2, s2, l3 = t3.length, f2 = r3.length, h2 = e, g2 = y(r3.length), c3 = r3[f2 - 1], m3 = Math.ceil(h2 / (2 * c3)), d3 = I(t3, m3), b2 = I(r3, m3);
          for (d3.length <= l3 && d3.push(0), b2.push(0), c3 = b2[f2 - 1], n3 = l3 - f2; n3 >= 0; n3--) {
            for (o3 = h2 - 1, d3[n3 + f2] !== c3 && (o3 = Math.floor((d3[n3 + f2] * h2 + d3[n3 + f2 - 1]) / c3)), i2 = 0, u2 = 0, a2 = b2.length, p2 = 0; p2 < a2; p2++)
              i2 += o3 * b2[p2], s2 = Math.floor(i2 / h2), u2 += d3[n3 + p2] - (i2 - s2 * h2), i2 = s2, u2 < 0 ? (d3[n3 + p2] = u2 + h2, u2 = -1) : (d3[n3 + p2] = u2, u2 = 0);
            for (; 0 !== u2; ) {
              for (o3 -= 1, i2 = 0, p2 = 0; p2 < a2; p2++)
                (i2 += d3[n3 + p2] - h2 + b2[p2]) < 0 ? (d3[n3 + p2] = i2 + h2, i2 = 0) : (d3[n3 + p2] = i2, i2 = 1);
              u2 += i2;
            }
            g2[n3] = o3;
          }
          return d3 = O(d3, m3)[0], [v(g2), v(d3)];
        }(c2, m2) : function(t3, r3) {
          for (var o3, n3, i2, u2, p2, a2 = t3.length, s2 = r3.length, l3 = [], f2 = [], y2 = e; a2; )
            if (f2.unshift(t3[--a2]), h(f2), A(f2, r3) < 0)
              l3.push(0);
            else {
              i2 = f2[(n3 = f2.length) - 1] * y2 + f2[n3 - 2], u2 = r3[s2 - 1] * y2 + r3[s2 - 2], n3 > s2 && (i2 = (i2 + 1) * y2), o3 = Math.ceil(i2 / u2);
              do {
                if (A(p2 = I(r3, o3), f2) <= 0)
                  break;
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
        if (t2.length !== e2.length)
          return t2.length > e2.length ? 1 : -1;
        for (var r2 = t2.length - 1; r2 >= 0; r2--)
          if (t2[r2] !== e2[r2])
            return t2[r2] > e2[r2] ? 1 : -1;
        return 0;
      }
      function P(t2) {
        var e2 = t2.abs();
        return !e2.isUnit() && (!!(e2.equals(2) || e2.equals(3) || e2.equals(5)) || !(e2.isEven() || e2.isDivisibleBy(3) || e2.isDivisibleBy(5)) && (!!e2.lesser(49) || void 0));
      }
      function Z(t2, e2) {
        for (var r2, o2, n2, i2 = t2.prev(), u2 = i2, p2 = 0; u2.isEven(); )
          u2 = u2.divide(2), p2++;
        t:
          for (o2 = 0; o2 < e2.length; o2++)
            if (!t2.lesser(e2[o2]) && !(n2 = bigInt2(e2[o2]).modPow(u2, t2)).isUnit() && !n2.equals(i2)) {
              for (r2 = p2 - 1; 0 != r2; r2--) {
                if ((n2 = n2.square().mod(t2)).isUnit())
                  return false;
                if (n2.equals(i2))
                  continue t;
              }
              return false;
            }
        return true;
      }
      p.prototype = Object.create(u.prototype), a.prototype = Object.create(u.prototype), s.prototype = Object.create(u.prototype), p.prototype.add = function(t2) {
        var e2 = K(t2);
        if (this.sign !== e2.sign)
          return this.subtract(e2.negate());
        var r2 = this.value, o2 = e2.value;
        return e2.isSmall ? new p(d(r2, Math.abs(o2)), this.sign) : new p(m(r2, o2), this.sign);
      }, p.prototype.plus = p.prototype.add, a.prototype.add = function(t2) {
        var e2 = K(t2), r2 = this.value;
        if (r2 < 0 !== e2.sign)
          return this.subtract(e2.negate());
        var o2 = e2.value;
        if (e2.isSmall) {
          if (l(r2 + o2))
            return new a(r2 + o2);
          o2 = f(Math.abs(o2));
        }
        return new p(d(o2, Math.abs(r2)), r2 < 0);
      }, a.prototype.plus = a.prototype.add, s.prototype.add = function(t2) {
        return new s(this.value + K(t2).value);
      }, s.prototype.plus = s.prototype.add, p.prototype.subtract = function(t2) {
        var e2 = K(t2);
        if (this.sign !== e2.sign)
          return this.add(e2.negate());
        var r2 = this.value, o2 = e2.value;
        return e2.isSmall ? w(r2, Math.abs(o2), this.sign) : function(t3, e3, r3) {
          var o3;
          return A(t3, e3) >= 0 ? o3 = b(t3, e3) : (o3 = b(e3, t3), r3 = !r3), "number" == typeof (o3 = v(o3)) ? (r3 && (o3 = -o3), new a(o3)) : new p(o3, r3);
        }(r2, o2, this.sign);
      }, p.prototype.minus = p.prototype.subtract, a.prototype.subtract = function(t2) {
        var e2 = K(t2), r2 = this.value;
        if (r2 < 0 !== e2.sign)
          return this.add(e2.negate());
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
          if (0 === s2)
            return u[0];
          if (1 === s2)
            return this;
          if (-1 === s2)
            return this.negate();
          if ((r2 = Math.abs(s2)) < e)
            return new p(I(a2, r2), l2);
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
        if (0 === p2)
          return u[1];
        if (0 === i2)
          return u[0];
        if (1 === i2)
          return u[1];
        if (-1 === i2)
          return n2.isEven() ? u[1] : u[-1];
        if (n2.sign)
          return u[0];
        if (!n2.isSmall)
          throw new Error("The exponent " + n2.toString() + " is too large.");
        if (this.isSmall && l(e2 = Math.pow(i2, p2)))
          return new a(g(e2));
        for (r2 = this, o2 = u[1]; true & p2 && (o2 = o2.times(r2), --p2), 0 !== p2; )
          p2 /= 2, r2 = r2.square();
        return o2;
      }, a.prototype.pow = p.prototype.pow, s.prototype.pow = function(t2) {
        var e2 = K(t2), r2 = this.value, o2 = e2.value, n2 = BigInt(0), i2 = BigInt(1), p2 = BigInt(2);
        if (o2 === n2)
          return u[1];
        if (r2 === n2)
          return u[0];
        if (r2 === i2)
          return u[1];
        if (r2 === BigInt(-1))
          return e2.isEven() ? u[1] : u[-1];
        if (e2.isNegative())
          return new s(n2);
        for (var a2 = this, l2 = u[1]; (o2 & i2) === i2 && (l2 = l2.times(a2), --o2), o2 !== n2; )
          o2 /= p2, a2 = a2.square();
        return l2;
      }, p.prototype.modPow = function(t2, e2) {
        if (t2 = K(t2), (e2 = K(e2)).isZero())
          throw new Error("Cannot take modPow with modulus 0");
        var r2 = u[1], o2 = this.mod(e2);
        for (t2.isNegative() && (t2 = t2.multiply(u[-1]), o2 = o2.modInv(e2)); t2.isPositive(); ) {
          if (o2.isZero())
            return u[0];
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
        if (t2 === 1 / 0)
          return -1;
        if (t2 === -1 / 0)
          return 1;
        var e2 = K(t2), r2 = this.value, o2 = e2.value;
        return this.sign !== e2.sign ? e2.sign ? 1 : -1 : e2.isSmall ? this.sign ? -1 : 1 : A(r2, o2) * (this.sign ? -1 : 1);
      }, p.prototype.compareTo = p.prototype.compare, a.prototype.compare = function(t2) {
        if (t2 === 1 / 0)
          return -1;
        if (t2 === -1 / 0)
          return 1;
        var e2 = K(t2), r2 = this.value, o2 = e2.value;
        return e2.isSmall ? r2 == o2 ? 0 : r2 > o2 ? 1 : -1 : r2 < 0 !== e2.sign ? r2 < 0 ? -1 : 1 : r2 < 0 ? 1 : -1;
      }, a.prototype.compareTo = a.prototype.compare, s.prototype.compare = function(t2) {
        if (t2 === 1 / 0)
          return -1;
        if (t2 === -1 / 0)
          return 1;
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
        if (r2 !== t)
          return r2;
        var o2 = this.abs(), n2 = o2.bitLength();
        if (n2 <= 64)
          return Z(o2, [2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37]);
        for (var i2 = Math.log(2) * n2.toJSNumber(), u2 = Math.ceil(true === e2 ? 2 * Math.pow(i2, 2) : i2), p2 = [], a2 = 0; a2 < u2; a2++)
          p2.push(bigInt2(a2 + 2));
        return Z(o2, p2);
      }, s.prototype.isPrime = a.prototype.isPrime = p.prototype.isPrime, p.prototype.isProbablePrime = function(e2, r2) {
        var o2 = P(this);
        if (o2 !== t)
          return o2;
        for (var n2 = this.abs(), i2 = e2 === t ? 5 : e2, u2 = [], p2 = 0; p2 < i2; p2++)
          u2.push(bigInt2.randBetween(2, n2.minus(2), r2));
        return Z(n2, u2);
      }, s.prototype.isProbablePrime = a.prototype.isProbablePrime = p.prototype.isProbablePrime, p.prototype.modInv = function(t2) {
        for (var e2, r2, o2, n2 = bigInt2.zero, i2 = bigInt2.one, u2 = K(t2), p2 = this.abs(); !p2.isZero(); )
          e2 = u2.divide(p2), r2 = n2, o2 = u2, n2 = i2, u2 = p2, i2 = r2.subtract(e2.multiply(i2)), p2 = o2.subtract(e2.multiply(p2));
        if (!u2.isUnit())
          throw new Error(this.toString() + " and " + t2.toString() + " are not co-prime");
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
      for (var x = [1]; 2 * x[x.length - 1] <= e; )
        x.push(2 * x[x.length - 1]);
      var J = x.length, L = x[J - 1];
      function U(t2) {
        return Math.abs(t2) <= e;
      }
      function T(t2, e2, r2) {
        e2 = K(e2);
        for (var o2 = t2.isNegative(), n2 = e2.isNegative(), i2 = o2 ? t2.not() : t2, u2 = n2 ? e2.not() : e2, p2 = 0, a2 = 0, s2 = null, l2 = null, f2 = []; !i2.isZero() || !u2.isZero(); )
          p2 = (s2 = B(i2, L))[1].toJSNumber(), o2 && (p2 = L - 1 - p2), a2 = (l2 = B(u2, L))[1].toJSNumber(), n2 && (a2 = L - 1 - a2), i2 = s2[0], u2 = l2[0], f2.push(r2(p2, a2));
        for (var v2 = 0 !== r2(o2 ? 1 : 0, n2 ? 1 : 0) ? bigInt2(-1) : bigInt2(0), h2 = f2.length - 1; h2 >= 0; h2 -= 1)
          v2 = v2.multiply(L).add(bigInt2(f2[h2]));
        return v2;
      }
      p.prototype.shiftLeft = function(t2) {
        var e2 = K(t2).toJSNumber();
        if (!U(e2))
          throw new Error(String(e2) + " is too large for shifting.");
        if (e2 < 0)
          return this.shiftRight(-e2);
        var r2 = this;
        if (r2.isZero())
          return r2;
        for (; e2 >= J; )
          r2 = r2.multiply(L), e2 -= J - 1;
        return r2.multiply(x[e2]);
      }, s.prototype.shiftLeft = a.prototype.shiftLeft = p.prototype.shiftLeft, p.prototype.shiftRight = function(t2) {
        var e2, r2 = K(t2).toJSNumber();
        if (!U(r2))
          throw new Error(String(r2) + " is too large for shifting.");
        if (r2 < 0)
          return this.shiftLeft(-r2);
        for (var o2 = this; r2 >= J; ) {
          if (o2.isZero() || o2.isNegative() && o2.isUnit())
            return o2;
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
        if (t2 = K(t2).abs(), e2 = K(e2).abs(), t2.equals(e2))
          return t2;
        if (t2.isZero())
          return e2;
        if (e2.isZero())
          return t2;
        for (var r2, o2, n2 = u[1]; t2.isEven() && e2.isEven(); )
          r2 = R(C(t2), C(e2)), t2 = t2.divide(r2), e2 = e2.divide(r2), n2 = n2.multiply(r2);
        for (; t2.isEven(); )
          t2 = t2.divide(C(t2));
        do {
          for (; e2.isEven(); )
            e2 = e2.divide(C(e2));
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
        for (i2 = 0; i2 < r2.length; i2++)
          a2[r2[i2]] = i2;
        for (i2 = 0; i2 < u2; i2++) {
          if ("-" !== (f2 = t2[i2]) && (f2 in a2 && a2[f2] >= p2)) {
            if ("1" === f2 && 1 === p2)
              continue;
            throw new Error(f2 + " is not a valid digit in base " + e2 + ".");
          }
        }
        e2 = K(e2);
        var s2 = [], l2 = "-" === t2[0];
        for (i2 = l2 ? 1 : 0; i2 < t2.length; i2++) {
          var f2;
          if ((f2 = t2[i2]) in a2)
            s2.push(K(a2[f2]));
          else {
            if ("<" !== f2)
              throw new Error(f2 + " is not a valid character");
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
        for (o2 = t2.length - 1; o2 >= 0; o2--)
          n2 = n2.add(t2[o2].times(i2)), i2 = i2.times(e2);
        return r2 ? n2.negate() : n2;
      }
      function F(t2, e2) {
        if ((e2 = bigInt2(e2)).isZero()) {
          if (t2.isZero())
            return { value: [0], isNegative: false };
          throw new Error("Cannot convert nonzero numbers to base 0.");
        }
        if (e2.equals(-1)) {
          if (t2.isZero())
            return { value: [0], isNegative: false };
          if (t2.isNegative())
            return { value: [].concat.apply([], Array.apply(null, Array(-t2.toJSNumber())).map(Array.prototype.valueOf, [1, 0])), isNegative: false };
          var r2 = Array.apply(null, Array(t2.toJSNumber() - 1)).map(Array.prototype.valueOf, [0, 1]);
          return r2.unshift([1]), { value: [].concat.apply([], r2), isNegative: false };
        }
        var o2 = false;
        if (t2.isNegative() && e2.isPositive() && (o2 = true, t2 = t2.abs()), e2.isUnit())
          return t2.isZero() ? { value: [0], isNegative: false } : { value: Array.apply(null, Array(t2.toJSNumber())).map(Number.prototype.valueOf, 1), isNegative: o2 };
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
          if (e2 === g(e2))
            return i ? new s(BigInt(e2)) : new a(e2);
          throw new Error("Invalid integer: " + t2);
        }
        var r2 = "-" === t2[0];
        r2 && (t2 = t2.slice(1));
        var o2 = t2.split(/e/i);
        if (o2.length > 2)
          throw new Error("Invalid integer: " + o2.join("e"));
        if (2 === o2.length) {
          var n2 = o2[1];
          if ("+" === n2[0] && (n2 = n2.slice(1)), (n2 = +n2) !== g(n2) || !l(n2))
            throw new Error("Invalid integer: " + n2 + " is not a valid exponent.");
          var u2 = o2[0], f2 = u2.indexOf(".");
          if (f2 >= 0 && (n2 -= u2.length - f2 - 1, u2 = u2.slice(0, f2) + u2.slice(f2 + 1)), n2 < 0)
            throw new Error("Cannot include negative exponent part for integers");
          t2 = u2 += new Array(n2 + 1).join("0");
        }
        if (!/^([0-9][0-9]*)$/.test(t2))
          throw new Error("Invalid integer: " + t2);
        if (i)
          return new s(BigInt(r2 ? "-" + t2 : t2));
        for (var v2 = [], y2 = t2.length, c2 = y2 - 7; y2 > 0; )
          v2.push(+t2.slice(c2, y2)), (c2 -= 7) < 0 && (c2 = 0), y2 -= 7;
        return h(v2), new p(v2, r2);
      }
      function K(t2) {
        return "number" == typeof t2 ? function(t3) {
          if (i)
            return new s(BigInt(t3));
          if (l(t3)) {
            if (t3 !== g(t3))
              throw new Error(t3 + " is not an integer.");
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
        if (e2 === t && (e2 = 10), 10 !== e2)
          return G(this, e2, r2);
        for (var o2, n2 = this.value, i2 = n2.length, u2 = String(n2[--i2]); --i2 >= 0; )
          o2 = String(n2[i2]), u2 += "0000000".slice(o2.length) + o2;
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
      for (var Q = 0; Q < 1e3; Q++)
        u[Q] = K(Q), Q > 0 && (u[-Q] = K(-Q));
      return u.one = u[1], u.zero = u[0], u.minusOne = u[-1], u.max = z, u.min = R, u.gcd = k, u.lcm = function(t2, e2) {
        return t2 = K(t2).abs(), e2 = K(e2).abs(), t2.divide(k(t2, e2)).multiply(e2);
      }, u.isInstance = function(t2) {
        return t2 instanceof p || t2 instanceof a || t2 instanceof s;
      }, u.randBetween = function(t2, r2, o2) {
        t2 = K(t2), r2 = K(r2);
        var n2 = o2 || Math.random, i2 = R(t2, r2), p2 = z(t2, r2).subtract(i2).add(1);
        if (p2.isSmall)
          return i2.add(Math.floor(n2() * p2));
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

// index.ts
var import_bigInt = __toESM(require_bigInt());

// node_modules/@bufbuild/protobuf/dist/esm/private/assert.js
function assert(condition, msg) {
  if (!condition) {
    throw new Error(msg);
  }
}
var FLOAT32_MAX = 34028234663852886e22;
var FLOAT32_MIN = -34028234663852886e22;
var UINT32_MAX = 4294967295;
var INT32_MAX = 2147483647;
var INT32_MIN = -2147483648;
function assertInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid int 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > INT32_MAX || arg < INT32_MIN)
    throw new Error("invalid int 32: " + arg);
}
function assertUInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid uint 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > UINT32_MAX || arg < 0)
    throw new Error("invalid uint 32: " + arg);
}
function assertFloat32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid float 32: " + typeof arg);
  if (!Number.isFinite(arg))
    return;
  if (arg > FLOAT32_MAX || arg < FLOAT32_MIN)
    throw new Error("invalid float 32: " + arg);
}

// node_modules/@bufbuild/protobuf/dist/esm/private/enum.js
var enumTypeSymbol = Symbol("@bufbuild/protobuf/enum-type");
function getEnumType(enumObject) {
  const t = enumObject[enumTypeSymbol];
  assert(t, "missing enum type on enum object");
  return t;
}
function setEnumType(enumObject, typeName, values, opt) {
  enumObject[enumTypeSymbol] = makeEnumType(typeName, values.map((v) => ({
    no: v.no,
    name: v.name,
    localName: enumObject[v.no]
  })), opt);
}
function makeEnumType(typeName, values, _opt) {
  const names = /* @__PURE__ */ Object.create(null);
  const numbers = /* @__PURE__ */ Object.create(null);
  const normalValues = [];
  for (const value of values) {
    const n = normalizeEnumValue(value);
    normalValues.push(n);
    names[value.name] = n;
    numbers[value.no] = n;
  }
  return {
    typeName,
    values: normalValues,
    // We do not surface options at this time
    // options: opt?.options ?? Object.create(null),
    findName(name) {
      return names[name];
    },
    findNumber(no) {
      return numbers[no];
    }
  };
}
function makeEnum(typeName, values, opt) {
  const enumObject = {};
  for (const value of values) {
    const n = normalizeEnumValue(value);
    enumObject[n.localName] = n.no;
    enumObject[n.no] = n.localName;
  }
  setEnumType(enumObject, typeName, values, opt);
  return enumObject;
}
function normalizeEnumValue(value) {
  if ("localName" in value) {
    return value;
  }
  return Object.assign(Object.assign({}, value), { localName: value.name });
}

// node_modules/@bufbuild/protobuf/dist/esm/message.js
var Message = class {
  /**
   * Compare with a message of the same type.
   */
  equals(other) {
    return this.getType().runtime.util.equals(this.getType(), this, other);
  }
  /**
   * Create a deep copy.
   */
  clone() {
    return this.getType().runtime.util.clone(this);
  }
  /**
   * Parse from binary data, merging fields.
   *
   * Repeated fields are appended. Map entries are added, overwriting
   * existing keys.
   *
   * If a message field is already present, it will be merged with the
   * new data.
   */
  fromBinary(bytes, options) {
    const type = this.getType(), format = type.runtime.bin, opt = format.makeReadOptions(options);
    format.readMessage(this, opt.readerFactory(bytes), bytes.byteLength, opt);
    return this;
  }
  /**
   * Parse a message from a JSON value.
   */
  fromJson(jsonValue, options) {
    const type = this.getType(), format = type.runtime.json, opt = format.makeReadOptions(options);
    format.readMessage(type, jsonValue, opt, this);
    return this;
  }
  /**
   * Parse a message from a JSON string.
   */
  fromJsonString(jsonString, options) {
    let json;
    try {
      json = JSON.parse(jsonString);
    } catch (e) {
      throw new Error(`cannot decode ${this.getType().typeName} from JSON: ${e instanceof Error ? e.message : String(e)}`);
    }
    return this.fromJson(json, options);
  }
  /**
   * Serialize the message to binary data.
   */
  toBinary(options) {
    const type = this.getType(), bin = type.runtime.bin, opt = bin.makeWriteOptions(options), writer = opt.writerFactory();
    bin.writeMessage(this, writer, opt);
    return writer.finish();
  }
  /**
   * Serialize the message to a JSON value, a JavaScript value that can be
   * passed to JSON.stringify().
   */
  toJson(options) {
    const type = this.getType(), json = type.runtime.json, opt = json.makeWriteOptions(options);
    return json.writeMessage(this, opt);
  }
  /**
   * Serialize the message to a JSON string.
   */
  toJsonString(options) {
    var _a;
    const value = this.toJson(options);
    return JSON.stringify(value, null, (_a = options === null || options === void 0 ? void 0 : options.prettySpaces) !== null && _a !== void 0 ? _a : 0);
  }
  /**
   * Override for serialization behavior. This will be invoked when calling
   * JSON.stringify on this message (i.e. JSON.stringify(msg)).
   *
   * Note that this will not serialize google.protobuf.Any with a packed
   * message because the protobuf JSON format specifies that it needs to be
   * unpacked, and this is only possible with a type registry to look up the
   * message type.  As a result, attempting to serialize a message with this
   * type will throw an Error.
   *
   * This method is protected because you should not need to invoke it
   * directly -- instead use JSON.stringify or toJsonString for
   * stringified JSON.  Alternatively, if actual JSON is desired, you should
   * use toJson.
   */
  toJSON() {
    return this.toJson({
      emitDefaultValues: true
    });
  }
  /**
   * Retrieve the MessageType of this message - a singleton that represents
   * the protobuf message declaration and provides metadata for reflection-
   * based operations.
   */
  getType() {
    return Object.getPrototypeOf(this).constructor;
  }
};

// node_modules/@bufbuild/protobuf/dist/esm/private/message-type.js
function makeMessageType(runtime, typeName, fields, opt) {
  var _a;
  const localName = (_a = opt === null || opt === void 0 ? void 0 : opt.localName) !== null && _a !== void 0 ? _a : typeName.substring(typeName.lastIndexOf(".") + 1);
  const type = {
    [localName]: function(data) {
      runtime.util.initFields(this);
      runtime.util.initPartial(data, this);
    }
  }[localName];
  Object.setPrototypeOf(type.prototype, new Message());
  Object.assign(type, {
    runtime,
    typeName,
    fields: runtime.util.newFieldList(fields),
    fromBinary(bytes, options) {
      return new type().fromBinary(bytes, options);
    },
    fromJson(jsonValue, options) {
      return new type().fromJson(jsonValue, options);
    },
    fromJsonString(jsonString, options) {
      return new type().fromJsonString(jsonString, options);
    },
    equals(a, b) {
      return runtime.util.equals(type, a, b);
    }
  });
  return type;
}

// node_modules/@bufbuild/protobuf/dist/esm/private/proto-runtime.js
function makeProtoRuntime(syntax, json, bin, util) {
  return {
    syntax,
    json,
    bin,
    util,
    makeMessageType(typeName, fields, opt) {
      return makeMessageType(this, typeName, fields, opt);
    },
    makeEnum,
    makeEnumType,
    getEnumType
  };
}

// node_modules/@bufbuild/protobuf/dist/esm/field.js
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

// node_modules/@bufbuild/protobuf/dist/esm/google/varint.js
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
function varint64write(lo, hi, bytes) {
  for (let i = 0; i < 28; i = i + 7) {
    const shift = lo >>> i;
    const hasNext = !(shift >>> 7 == 0 && hi == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  const splitBits = lo >>> 28 & 15 | (hi & 7) << 4;
  const hasMoreBits = !(hi >> 3 == 0);
  bytes.push((hasMoreBits ? splitBits | 128 : splitBits) & 255);
  if (!hasMoreBits) {
    return;
  }
  for (let i = 3; i < 31; i = i + 7) {
    const shift = hi >>> i;
    const hasNext = !(shift >>> 7 == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  bytes.push(hi >>> 31 & 1);
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
function varint32write(value, bytes) {
  if (value >= 0) {
    while (value > 127) {
      bytes.push(value & 127 | 128);
      value = value >>> 7;
    }
    bytes.push(value);
  } else {
    for (let i = 0; i < 9; i++) {
      bytes.push(value & 127 | 128);
      value = value >> 7;
    }
    bytes.push(1);
  }
}
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
function makeInt64Support() {
  const dv = new DataView(new ArrayBuffer(8));
  const ok = globalThis.BigInt !== void 0 && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function" && (typeof process != "object" || typeof process.env != "object" || process.env.BUF_BIGINT_DISABLE !== "1");
  if (ok) {
    const MIN = BigInt("-9223372036854775808"), MAX = BigInt("9223372036854775807"), UMIN = BigInt("0"), UMAX = BigInt("18446744073709551615");
    return {
      zero: BigInt(0),
      supported: true,
      parse(value) {
        const bi = typeof value == "bigint" ? value : BigInt(value);
        if (bi > MAX || bi < MIN) {
          throw new Error(`int64 invalid: ${value}`);
        }
        return bi;
      },
      uParse(value) {
        const bi = typeof value == "bigint" ? value : BigInt(value);
        if (bi > UMAX || bi < UMIN) {
          throw new Error(`uint64 invalid: ${value}`);
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
  const assertInt64String = (value) => assert(/^-?[0-9]+$/.test(value), `int64 invalid: ${value}`);
  const assertUInt64String = (value) => assert(/^[0-9]+$/.test(value), `uint64 invalid: ${value}`);
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
var protoInt64 = makeInt64Support();

// node_modules/@bufbuild/protobuf/dist/esm/binary-encoding.js
var WireType;
(function(WireType2) {
  WireType2[WireType2["Varint"] = 0] = "Varint";
  WireType2[WireType2["Bit64"] = 1] = "Bit64";
  WireType2[WireType2["LengthDelimited"] = 2] = "LengthDelimited";
  WireType2[WireType2["StartGroup"] = 3] = "StartGroup";
  WireType2[WireType2["EndGroup"] = 4] = "EndGroup";
  WireType2[WireType2["Bit32"] = 5] = "Bit32";
})(WireType || (WireType = {}));
var BinaryWriter = class {
  constructor(textEncoder) {
    this.stack = [];
    this.textEncoder = textEncoder !== null && textEncoder !== void 0 ? textEncoder : new TextEncoder();
    this.chunks = [];
    this.buf = [];
  }
  /**
   * Return all bytes written and reset this writer.
   */
  finish() {
    this.chunks.push(new Uint8Array(this.buf));
    let len = 0;
    for (let i = 0; i < this.chunks.length; i++)
      len += this.chunks[i].length;
    let bytes = new Uint8Array(len);
    let offset = 0;
    for (let i = 0; i < this.chunks.length; i++) {
      bytes.set(this.chunks[i], offset);
      offset += this.chunks[i].length;
    }
    this.chunks = [];
    return bytes;
  }
  /**
   * Start a new fork for length-delimited data like a message
   * or a packed repeated field.
   *
   * Must be joined later with `join()`.
   */
  fork() {
    this.stack.push({ chunks: this.chunks, buf: this.buf });
    this.chunks = [];
    this.buf = [];
    return this;
  }
  /**
   * Join the last fork. Write its length and bytes, then
   * return to the previous state.
   */
  join() {
    let chunk = this.finish();
    let prev = this.stack.pop();
    if (!prev)
      throw new Error("invalid state, fork stack empty");
    this.chunks = prev.chunks;
    this.buf = prev.buf;
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Writes a tag (field number and wire type).
   *
   * Equivalent to `uint32( (fieldNo << 3 | type) >>> 0 )`.
   *
   * Generated code should compute the tag ahead of time and call `uint32()`.
   */
  tag(fieldNo, type) {
    return this.uint32((fieldNo << 3 | type) >>> 0);
  }
  /**
   * Write a chunk of raw bytes.
   */
  raw(chunk) {
    if (this.buf.length) {
      this.chunks.push(new Uint8Array(this.buf));
      this.buf = [];
    }
    this.chunks.push(chunk);
    return this;
  }
  /**
   * Write a `uint32` value, an unsigned 32 bit varint.
   */
  uint32(value) {
    assertUInt32(value);
    while (value > 127) {
      this.buf.push(value & 127 | 128);
      value = value >>> 7;
    }
    this.buf.push(value);
    return this;
  }
  /**
   * Write a `int32` value, a signed 32 bit varint.
   */
  int32(value) {
    assertInt32(value);
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `bool` value, a variant.
   */
  bool(value) {
    this.buf.push(value ? 1 : 0);
    return this;
  }
  /**
   * Write a `bytes` value, length-delimited arbitrary data.
   */
  bytes(value) {
    this.uint32(value.byteLength);
    return this.raw(value);
  }
  /**
   * Write a `string` value, length-delimited data converted to UTF-8 text.
   */
  string(value) {
    let chunk = this.textEncoder.encode(value);
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Write a `float` value, 32-bit floating point number.
   */
  float(value) {
    assertFloat32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setFloat32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `double` value, a 64-bit floating point number.
   */
  double(value) {
    let chunk = new Uint8Array(8);
    new DataView(chunk.buffer).setFloat64(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed32` value, an unsigned, fixed-length 32-bit integer.
   */
  fixed32(value) {
    assertUInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setUint32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sfixed32` value, a signed, fixed-length 32-bit integer.
   */
  sfixed32(value) {
    assertInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setInt32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sint32` value, a signed, zigzag-encoded 32-bit varint.
   */
  sint32(value) {
    assertInt32(value);
    value = (value << 1 ^ value >> 31) >>> 0;
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `fixed64` value, a signed, fixed-length 64-bit integer.
   */
  sfixed64(value) {
    let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = protoInt64.enc(value);
    view.setInt32(0, tc.lo, true);
    view.setInt32(4, tc.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed64` value, an unsigned, fixed-length 64 bit integer.
   */
  fixed64(value) {
    let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = protoInt64.uEnc(value);
    view.setInt32(0, tc.lo, true);
    view.setInt32(4, tc.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `int64` value, a signed 64-bit varint.
   */
  int64(value) {
    let tc = protoInt64.enc(value);
    varint64write(tc.lo, tc.hi, this.buf);
    return this;
  }
  /**
   * Write a `sint64` value, a signed, zig-zag-encoded 64-bit varint.
   */
  sint64(value) {
    let tc = protoInt64.enc(value), sign = tc.hi >> 31, lo = tc.lo << 1 ^ sign, hi = (tc.hi << 1 | tc.lo >>> 31) ^ sign;
    varint64write(lo, hi, this.buf);
    return this;
  }
  /**
   * Write a `uint64` value, an unsigned 64-bit varint.
   */
  uint64(value) {
    let tc = protoInt64.uEnc(value);
    varint64write(tc.lo, tc.hi, this.buf);
    return this;
  }
};
var BinaryReader = class {
  constructor(buf, textDecoder) {
    this.varint64 = varint64read;
    this.uint32 = varint32read;
    this.buf = buf;
    this.len = buf.length;
    this.pos = 0;
    this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    this.textDecoder = textDecoder !== null && textDecoder !== void 0 ? textDecoder : new TextDecoder();
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
   * Skip one element on the wire and return the skipped data.
   * Supports WireType.StartGroup since v2.0.0-alpha.23.
   */
  skip(wireType) {
    let start = this.pos;
    switch (wireType) {
      case WireType.Varint:
        while (this.buf[this.pos++] & 128) {
        }
        break;
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
        let t;
        while ((t = this.tag()[1]) !== WireType.EndGroup) {
          this.skip(t);
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
    return this.textDecoder.decode(this.bytes());
  }
};

// node_modules/@bufbuild/protobuf/dist/esm/private/field-wrapper.js
function wrapField(type, value) {
  if (value instanceof Message || !type.fieldWrapper) {
    return value;
  }
  return type.fieldWrapper.wrapField(value);
}
var wktWrapperToScalarType = {
  "google.protobuf.DoubleValue": ScalarType.DOUBLE,
  "google.protobuf.FloatValue": ScalarType.FLOAT,
  "google.protobuf.Int64Value": ScalarType.INT64,
  "google.protobuf.UInt64Value": ScalarType.UINT64,
  "google.protobuf.Int32Value": ScalarType.INT32,
  "google.protobuf.UInt32Value": ScalarType.UINT32,
  "google.protobuf.BoolValue": ScalarType.BOOL,
  "google.protobuf.StringValue": ScalarType.STRING,
  "google.protobuf.BytesValue": ScalarType.BYTES
};

// node_modules/@bufbuild/protobuf/dist/esm/private/scalars.js
function scalarEquals(type, a, b) {
  if (a === b) {
    return true;
  }
  if (type == ScalarType.BYTES) {
    if (!(a instanceof Uint8Array) || !(b instanceof Uint8Array)) {
      return false;
    }
    if (a.length !== b.length) {
      return false;
    }
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) {
        return false;
      }
    }
    return true;
  }
  switch (type) {
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      return a == b;
  }
  return false;
}
function scalarDefaultValue(type) {
  switch (type) {
    case ScalarType.BOOL:
      return false;
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      return protoInt64.zero;
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT:
      return 0;
    case ScalarType.BYTES:
      return new Uint8Array(0);
    case ScalarType.STRING:
      return "";
    default:
      return 0;
  }
}
function scalarTypeInfo(type, value) {
  const isUndefined = value === void 0;
  let wireType = WireType.Varint;
  let isIntrinsicDefault = value === 0;
  switch (type) {
    case ScalarType.STRING:
      isIntrinsicDefault = isUndefined || !value.length;
      wireType = WireType.LengthDelimited;
      break;
    case ScalarType.BOOL:
      isIntrinsicDefault = value === false;
      break;
    case ScalarType.DOUBLE:
      wireType = WireType.Bit64;
      break;
    case ScalarType.FLOAT:
      wireType = WireType.Bit32;
      break;
    case ScalarType.INT64:
      isIntrinsicDefault = isUndefined || value == 0;
      break;
    case ScalarType.UINT64:
      isIntrinsicDefault = isUndefined || value == 0;
      break;
    case ScalarType.FIXED64:
      isIntrinsicDefault = isUndefined || value == 0;
      wireType = WireType.Bit64;
      break;
    case ScalarType.BYTES:
      isIntrinsicDefault = isUndefined || !value.byteLength;
      wireType = WireType.LengthDelimited;
      break;
    case ScalarType.FIXED32:
      wireType = WireType.Bit32;
      break;
    case ScalarType.SFIXED32:
      wireType = WireType.Bit32;
      break;
    case ScalarType.SFIXED64:
      isIntrinsicDefault = isUndefined || value == 0;
      wireType = WireType.Bit64;
      break;
    case ScalarType.SINT64:
      isIntrinsicDefault = isUndefined || value == 0;
      break;
  }
  const method = ScalarType[type].toLowerCase();
  return [wireType, method, isUndefined || isIntrinsicDefault];
}

// node_modules/@bufbuild/protobuf/dist/esm/private/binary-format-common.js
var unknownFieldsSymbol = Symbol("@bufbuild/protobuf/unknown-fields");
var readDefaults = {
  readUnknownFields: true,
  readerFactory: (bytes) => new BinaryReader(bytes)
};
var writeDefaults = {
  writeUnknownFields: true,
  writerFactory: () => new BinaryWriter()
};
function makeReadOptions(options) {
  return options ? Object.assign(Object.assign({}, readDefaults), options) : readDefaults;
}
function makeWriteOptions(options) {
  return options ? Object.assign(Object.assign({}, writeDefaults), options) : writeDefaults;
}
function makeBinaryFormatCommon() {
  return {
    makeReadOptions,
    makeWriteOptions,
    listUnknownFields(message) {
      var _a;
      return (_a = message[unknownFieldsSymbol]) !== null && _a !== void 0 ? _a : [];
    },
    discardUnknownFields(message) {
      delete message[unknownFieldsSymbol];
    },
    writeUnknownFields(message, writer) {
      const m = message;
      const c = m[unknownFieldsSymbol];
      if (c) {
        for (const f of c) {
          writer.tag(f.no, f.wireType).raw(f.data);
        }
      }
    },
    onUnknownField(message, no, wireType, data) {
      const m = message;
      if (!Array.isArray(m[unknownFieldsSymbol])) {
        m[unknownFieldsSymbol] = [];
      }
      m[unknownFieldsSymbol].push({ no, wireType, data });
    },
    readMessage(message, reader, length, options) {
      const type = message.getType();
      const end = length === void 0 ? reader.len : reader.pos + length;
      while (reader.pos < end) {
        const [fieldNo, wireType] = reader.tag(), field = type.fields.find(fieldNo);
        if (!field) {
          const data = reader.skip(wireType);
          if (options.readUnknownFields) {
            this.onUnknownField(message, fieldNo, wireType, data);
          }
          continue;
        }
        let target = message, repeated = field.repeated, localName = field.localName;
        if (field.oneof) {
          target = target[field.oneof.localName];
          if (target.case != localName) {
            delete target.value;
          }
          target.case = localName;
          localName = "value";
        }
        switch (field.kind) {
          case "scalar":
          case "enum":
            const scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
            if (repeated) {
              let arr = target[localName];
              if (wireType == WireType.LengthDelimited && scalarType != ScalarType.STRING && scalarType != ScalarType.BYTES) {
                let e = reader.uint32() + reader.pos;
                while (reader.pos < e) {
                  arr.push(readScalar(reader, scalarType));
                }
              } else {
                arr.push(readScalar(reader, scalarType));
              }
            } else {
              target[localName] = readScalar(reader, scalarType);
            }
            break;
          case "message":
            const messageType = field.T;
            if (repeated) {
              target[localName].push(readMessageField(reader, new messageType(), options));
            } else {
              if (target[localName] instanceof Message) {
                readMessageField(reader, target[localName], options);
              } else {
                target[localName] = readMessageField(reader, new messageType(), options);
                if (messageType.fieldWrapper && !field.oneof && !field.repeated) {
                  target[localName] = messageType.fieldWrapper.unwrapField(target[localName]);
                }
              }
            }
            break;
          case "map":
            let [mapKey, mapVal] = readMapEntry(field, reader, options);
            target[localName][mapKey] = mapVal;
            break;
        }
      }
    }
  };
}
function readMessageField(reader, message, options) {
  const format = message.getType().runtime.bin;
  format.readMessage(message, reader, reader.uint32(), options);
  return message;
}
function readMapEntry(field, reader, options) {
  const length = reader.uint32(), end = reader.pos + length;
  let key, val;
  while (reader.pos < end) {
    let [fieldNo] = reader.tag();
    switch (fieldNo) {
      case 1:
        key = readScalar(reader, field.K);
        break;
      case 2:
        switch (field.V.kind) {
          case "scalar":
            val = readScalar(reader, field.V.T);
            break;
          case "enum":
            val = reader.int32();
            break;
          case "message":
            val = readMessageField(reader, new field.V.T(), options);
            break;
        }
        break;
    }
  }
  if (key === void 0) {
    let keyRaw = scalarDefaultValue(field.K);
    key = field.K == ScalarType.BOOL ? keyRaw.toString() : keyRaw;
  }
  if (typeof key != "string" && typeof key != "number") {
    key = key.toString();
  }
  if (val === void 0) {
    switch (field.V.kind) {
      case "scalar":
        val = scalarDefaultValue(field.V.T);
        break;
      case "enum":
        val = 0;
        break;
      case "message":
        val = new field.V.T();
        break;
    }
  }
  return [key, val];
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
function writeMapEntry(writer, options, field, key, value) {
  writer.tag(field.no, WireType.LengthDelimited);
  writer.fork();
  let keyValue = key;
  switch (field.K) {
    case ScalarType.INT32:
    case ScalarType.FIXED32:
    case ScalarType.UINT32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
      keyValue = Number.parseInt(key);
      break;
    case ScalarType.BOOL:
      assert(key == "true" || key == "false");
      keyValue = key == "true";
      break;
  }
  writeScalar(writer, field.K, 1, keyValue, true);
  switch (field.V.kind) {
    case "scalar":
      writeScalar(writer, field.V.T, 2, value, true);
      break;
    case "enum":
      writeScalar(writer, ScalarType.INT32, 2, value, true);
      break;
    case "message":
      writeMessageField(writer, options, field.V.T, 2, value);
      break;
  }
  writer.join();
}
function writeMessageField(writer, options, type, fieldNo, value) {
  if (value !== void 0) {
    const message = wrapField(type, value);
    writer.tag(fieldNo, WireType.LengthDelimited).bytes(message.toBinary(options));
  }
}
function writeScalar(writer, type, fieldNo, value, emitIntrinsicDefault) {
  let [wireType, method, isIntrinsicDefault] = scalarTypeInfo(type, value);
  if (!isIntrinsicDefault || emitIntrinsicDefault) {
    writer.tag(fieldNo, wireType)[method](value);
  }
}
function writePacked(writer, type, fieldNo, value) {
  if (!value.length) {
    return;
  }
  writer.tag(fieldNo, WireType.LengthDelimited).fork();
  let [, method] = scalarTypeInfo(type);
  for (let i = 0; i < value.length; i++) {
    writer[method](value[i]);
  }
  writer.join();
}

// node_modules/@bufbuild/protobuf/dist/esm/private/binary-format-proto3.js
function makeBinaryFormatProto3() {
  return Object.assign(Object.assign({}, makeBinaryFormatCommon()), { writeMessage(message, writer, options) {
    const type = message.getType();
    for (const field of type.fields.byNumber()) {
      let value, repeated = field.repeated, localName = field.localName;
      if (field.oneof) {
        const oneof = message[field.oneof.localName];
        if (oneof.case !== localName) {
          continue;
        }
        value = oneof.value;
      } else {
        value = message[localName];
      }
      switch (field.kind) {
        case "scalar":
        case "enum":
          let scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
          if (repeated) {
            if (field.packed) {
              writePacked(writer, scalarType, field.no, value);
            } else {
              for (const item of value) {
                writeScalar(writer, scalarType, field.no, item, true);
              }
            }
          } else {
            if (value !== void 0) {
              writeScalar(writer, scalarType, field.no, value, !!field.oneof || field.opt);
            }
          }
          break;
        case "message":
          if (repeated) {
            for (const item of value) {
              writeMessageField(writer, options, field.T, field.no, item);
            }
          } else {
            writeMessageField(writer, options, field.T, field.no, value);
          }
          break;
        case "map":
          for (const [key, val] of Object.entries(value)) {
            writeMapEntry(writer, options, field, key, val);
          }
          break;
      }
    }
    if (options.writeUnknownFields) {
      this.writeUnknownFields(message, writer);
    }
    return writer;
  } });
}

// node_modules/@bufbuild/protobuf/dist/esm/proto-base64.js
var encTable = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
var decTable = [];
for (let i = 0; i < encTable.length; i++)
  decTable[encTable[i].charCodeAt(0)] = i;
decTable["-".charCodeAt(0)] = encTable.indexOf("+");
decTable["_".charCodeAt(0)] = encTable.indexOf("/");
var protoBase64 = {
  /**
   * Decodes a base64 string to a byte array.
   *
   * - ignores white-space, including line breaks and tabs
   * - allows inner padding (can decode concatenated base64 strings)
   * - does not require padding
   * - understands base64url encoding:
   *   "-" instead of "+",
   *   "_" instead of "/",
   *   no padding
   */
  dec(base64Str) {
    let es = base64Str.length * 3 / 4;
    if (base64Str[base64Str.length - 2] == "=")
      es -= 2;
    else if (base64Str[base64Str.length - 1] == "=")
      es -= 1;
    let bytes = new Uint8Array(es), bytePos = 0, groupPos = 0, b, p = 0;
    for (let i = 0; i < base64Str.length; i++) {
      b = decTable[base64Str.charCodeAt(i)];
      if (b === void 0) {
        switch (base64Str[i]) {
          case "=":
            groupPos = 0;
          case "\n":
          case "\r":
          case "	":
          case " ":
            continue;
          default:
            throw Error("invalid base64 string.");
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
      throw Error("invalid base64 string.");
    return bytes.subarray(0, bytePos);
  },
  /**
   * Encode a byte array to a base64 string.
   */
  enc(bytes) {
    let base64 = "", groupPos = 0, b, p = 0;
    for (let i = 0; i < bytes.length; i++) {
      b = bytes[i];
      switch (groupPos) {
        case 0:
          base64 += encTable[b >> 2];
          p = (b & 3) << 4;
          groupPos = 1;
          break;
        case 1:
          base64 += encTable[p | b >> 4];
          p = (b & 15) << 2;
          groupPos = 2;
          break;
        case 2:
          base64 += encTable[p | b >> 6];
          base64 += encTable[b & 63];
          groupPos = 0;
          break;
      }
    }
    if (groupPos) {
      base64 += encTable[p];
      base64 += "=";
      if (groupPos == 1)
        base64 += "=";
    }
    return base64;
  }
};

// node_modules/@bufbuild/protobuf/dist/esm/private/json-format-common.js
var jsonReadDefaults = {
  ignoreUnknownFields: false
};
var jsonWriteDefaults = {
  emitDefaultValues: false,
  enumAsInteger: false,
  useProtoFieldName: false,
  prettySpaces: 0
};
function makeReadOptions2(options) {
  return options ? Object.assign(Object.assign({}, jsonReadDefaults), options) : jsonReadDefaults;
}
function makeWriteOptions2(options) {
  return options ? Object.assign(Object.assign({}, jsonWriteDefaults), options) : jsonWriteDefaults;
}
function makeJsonFormatCommon(makeWriteField) {
  const writeField = makeWriteField(writeEnum, writeScalar2);
  return {
    makeReadOptions: makeReadOptions2,
    makeWriteOptions: makeWriteOptions2,
    readMessage(type, json, options, message) {
      if (json == null || Array.isArray(json) || typeof json != "object") {
        throw new Error(`cannot decode message ${type.typeName} from JSON: ${this.debug(json)}`);
      }
      message = message !== null && message !== void 0 ? message : new type();
      const oneofSeen = {};
      for (const [jsonKey, jsonValue] of Object.entries(json)) {
        const field = type.fields.findJsonName(jsonKey);
        if (!field) {
          if (!options.ignoreUnknownFields) {
            throw new Error(`cannot decode message ${type.typeName} from JSON: key "${jsonKey}" is unknown`);
          }
          continue;
        }
        let localName = field.localName;
        let target = message;
        if (field.oneof) {
          if (jsonValue === null && field.kind == "scalar") {
            continue;
          }
          const seen = oneofSeen[field.oneof.localName];
          if (seen) {
            throw new Error(`cannot decode message ${type.typeName} from JSON: multiple keys for oneof "${field.oneof.name}" present: "${seen}", "${jsonKey}"`);
          }
          oneofSeen[field.oneof.localName] = jsonKey;
          target = target[field.oneof.localName] = { case: localName };
          localName = "value";
        }
        if (field.repeated) {
          if (jsonValue === null) {
            continue;
          }
          if (!Array.isArray(jsonValue)) {
            throw new Error(`cannot decode field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonValue)}`);
          }
          const targetArray = target[localName];
          for (const jsonItem of jsonValue) {
            if (jsonItem === null) {
              throw new Error(`cannot decode field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonItem)}`);
            }
            let val;
            switch (field.kind) {
              case "message":
                val = field.T.fromJson(jsonItem, options);
                break;
              case "enum":
                val = readEnum(field.T, jsonItem, options.ignoreUnknownFields);
                if (val === void 0)
                  continue;
                break;
              case "scalar":
                try {
                  val = readScalar2(field.T, jsonItem);
                } catch (e) {
                  let m = `cannot decode field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonItem)}`;
                  if (e instanceof Error && e.message.length > 0) {
                    m += `: ${e.message}`;
                  }
                  throw new Error(m);
                }
                break;
            }
            targetArray.push(val);
          }
        } else if (field.kind == "map") {
          if (jsonValue === null) {
            continue;
          }
          if (Array.isArray(jsonValue) || typeof jsonValue != "object") {
            throw new Error(`cannot decode field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonValue)}`);
          }
          const targetMap = target[localName];
          for (const [jsonMapKey, jsonMapValue] of Object.entries(jsonValue)) {
            if (jsonMapValue === null) {
              throw new Error(`cannot decode field ${type.typeName}.${field.name} from JSON: map value null`);
            }
            let val;
            switch (field.V.kind) {
              case "message":
                val = field.V.T.fromJson(jsonMapValue, options);
                break;
              case "enum":
                val = readEnum(field.V.T, jsonMapValue, options.ignoreUnknownFields);
                if (val === void 0)
                  continue;
                break;
              case "scalar":
                try {
                  val = readScalar2(field.V.T, jsonMapValue);
                } catch (e) {
                  let m = `cannot decode map value for field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonValue)}`;
                  if (e instanceof Error && e.message.length > 0) {
                    m += `: ${e.message}`;
                  }
                  throw new Error(m);
                }
                break;
            }
            try {
              targetMap[readScalar2(field.K, field.K == ScalarType.BOOL ? jsonMapKey == "true" ? true : jsonMapKey == "false" ? false : jsonMapKey : jsonMapKey).toString()] = val;
            } catch (e) {
              let m = `cannot decode map key for field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonValue)}`;
              if (e instanceof Error && e.message.length > 0) {
                m += `: ${e.message}`;
              }
              throw new Error(m);
            }
          }
        } else {
          switch (field.kind) {
            case "message":
              const messageType = field.T;
              if (jsonValue === null && messageType.typeName != "google.protobuf.Value") {
                if (field.oneof) {
                  throw new Error(`cannot decode field ${type.typeName}.${field.name} from JSON: null is invalid for oneof field "${jsonKey}"`);
                }
                continue;
              }
              if (target[localName] instanceof Message) {
                target[localName].fromJson(jsonValue, options);
              } else {
                target[localName] = messageType.fromJson(jsonValue, options);
                if (messageType.fieldWrapper && !field.oneof) {
                  target[localName] = messageType.fieldWrapper.unwrapField(target[localName]);
                }
              }
              break;
            case "enum":
              const enumValue = readEnum(field.T, jsonValue, options.ignoreUnknownFields);
              if (enumValue !== void 0) {
                target[localName] = enumValue;
              }
              break;
            case "scalar":
              try {
                target[localName] = readScalar2(field.T, jsonValue);
              } catch (e) {
                let m = `cannot decode field ${type.typeName}.${field.name} from JSON: ${this.debug(jsonValue)}`;
                if (e instanceof Error && e.message.length > 0) {
                  m += `: ${e.message}`;
                }
                throw new Error(m);
              }
              break;
          }
        }
      }
      return message;
    },
    writeMessage(message, options) {
      const type = message.getType();
      const json = {};
      let field;
      try {
        for (const member of type.fields.byMember()) {
          let jsonValue;
          if (member.kind == "oneof") {
            const oneof = message[member.localName];
            if (oneof.value === void 0) {
              continue;
            }
            field = member.findField(oneof.case);
            if (!field) {
              throw "oneof case not found: " + oneof.case;
            }
            jsonValue = writeField(field, oneof.value, options);
          } else {
            field = member;
            jsonValue = writeField(field, message[field.localName], options);
          }
          if (jsonValue !== void 0) {
            json[options.useProtoFieldName ? field.name : field.jsonName] = jsonValue;
          }
        }
      } catch (e) {
        const m = field ? `cannot encode field ${type.typeName}.${field.name} to JSON` : `cannot encode message ${type.typeName} to JSON`;
        const r = e instanceof Error ? e.message : String(e);
        throw new Error(m + (r.length > 0 ? `: ${r}` : ""));
      }
      return json;
    },
    readScalar: readScalar2,
    writeScalar: writeScalar2,
    debug: debugJsonValue
  };
}
function debugJsonValue(json) {
  if (json === null) {
    return "null";
  }
  switch (typeof json) {
    case "object":
      return Array.isArray(json) ? "array" : "object";
    case "string":
      return json.length > 100 ? "string" : `"${json.split('"').join('\\"')}"`;
    default:
      return json.toString();
  }
}
function readScalar2(type, json) {
  switch (type) {
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT:
      if (json === null)
        return 0;
      if (json === "NaN")
        return Number.NaN;
      if (json === "Infinity")
        return Number.POSITIVE_INFINITY;
      if (json === "-Infinity")
        return Number.NEGATIVE_INFINITY;
      if (json === "") {
        break;
      }
      if (typeof json == "string" && json.trim().length !== json.length) {
        break;
      }
      if (typeof json != "string" && typeof json != "number") {
        break;
      }
      const float = Number(json);
      if (Number.isNaN(float)) {
        break;
      }
      if (!Number.isFinite(float)) {
        break;
      }
      if (type == ScalarType.FLOAT)
        assertFloat32(float);
      return float;
    case ScalarType.INT32:
    case ScalarType.FIXED32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
    case ScalarType.UINT32:
      if (json === null)
        return 0;
      let int32;
      if (typeof json == "number")
        int32 = json;
      else if (typeof json == "string" && json.length > 0) {
        if (json.trim().length === json.length)
          int32 = Number(json);
      }
      if (int32 === void 0)
        break;
      if (type == ScalarType.UINT32)
        assertUInt32(int32);
      else
        assertInt32(int32);
      return int32;
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      if (json === null)
        return protoInt64.zero;
      if (typeof json != "number" && typeof json != "string")
        break;
      return protoInt64.parse(json);
    case ScalarType.FIXED64:
    case ScalarType.UINT64:
      if (json === null)
        return protoInt64.zero;
      if (typeof json != "number" && typeof json != "string")
        break;
      return protoInt64.uParse(json);
    case ScalarType.BOOL:
      if (json === null)
        return false;
      if (typeof json !== "boolean")
        break;
      return json;
    case ScalarType.STRING:
      if (json === null)
        return "";
      if (typeof json !== "string") {
        break;
      }
      try {
        encodeURIComponent(json);
      } catch (e) {
        throw new Error("invalid UTF8");
      }
      return json;
    case ScalarType.BYTES:
      if (json === null || json === "")
        return new Uint8Array(0);
      if (typeof json !== "string")
        break;
      return protoBase64.dec(json);
  }
  throw new Error();
}
function readEnum(type, json, ignoreUnknownFields) {
  if (json === null) {
    return 0;
  }
  switch (typeof json) {
    case "number":
      if (Number.isInteger(json)) {
        return json;
      }
      break;
    case "string":
      const value = type.findName(json);
      if (value || ignoreUnknownFields) {
        return value === null || value === void 0 ? void 0 : value.no;
      }
      break;
  }
  throw new Error(`cannot decode enum ${type.typeName} from JSON: ${debugJsonValue(json)}`);
}
function writeEnum(type, value, emitIntrinsicDefault, enumAsInteger) {
  var _a;
  if (value === void 0) {
    return value;
  }
  if (value === 0 && !emitIntrinsicDefault) {
    return void 0;
  }
  if (enumAsInteger) {
    return value;
  }
  if (type.typeName == "google.protobuf.NullValue") {
    return null;
  }
  const val = type.findNumber(value);
  return (_a = val === null || val === void 0 ? void 0 : val.name) !== null && _a !== void 0 ? _a : value;
}
function writeScalar2(type, value, emitIntrinsicDefault) {
  if (value === void 0) {
    return void 0;
  }
  switch (type) {
    case ScalarType.INT32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
    case ScalarType.FIXED32:
    case ScalarType.UINT32:
      assert(typeof value == "number");
      return value != 0 || emitIntrinsicDefault ? value : void 0;
    case ScalarType.FLOAT:
    case ScalarType.DOUBLE:
      assert(typeof value == "number");
      if (Number.isNaN(value))
        return "NaN";
      if (value === Number.POSITIVE_INFINITY)
        return "Infinity";
      if (value === Number.NEGATIVE_INFINITY)
        return "-Infinity";
      return value !== 0 || emitIntrinsicDefault ? value : void 0;
    case ScalarType.STRING:
      assert(typeof value == "string");
      return value.length > 0 || emitIntrinsicDefault ? value : void 0;
    case ScalarType.BOOL:
      assert(typeof value == "boolean");
      return value || emitIntrinsicDefault ? value : void 0;
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      assert(typeof value == "bigint" || typeof value == "string" || typeof value == "number");
      return emitIntrinsicDefault || value != 0 ? value.toString(10) : void 0;
    case ScalarType.BYTES:
      assert(value instanceof Uint8Array);
      return emitIntrinsicDefault || value.byteLength > 0 ? protoBase64.enc(value) : void 0;
  }
}

// node_modules/@bufbuild/protobuf/dist/esm/private/json-format-proto3.js
function makeJsonFormatProto3() {
  return makeJsonFormatCommon((writeEnum2, writeScalar3) => {
    return function writeField(field, value, options) {
      if (field.kind == "map") {
        const jsonObj = {};
        switch (field.V.kind) {
          case "scalar":
            for (const [entryKey, entryValue] of Object.entries(value)) {
              const val = writeScalar3(field.V.T, entryValue, true);
              assert(val !== void 0);
              jsonObj[entryKey.toString()] = val;
            }
            break;
          case "message":
            for (const [entryKey, entryValue] of Object.entries(value)) {
              jsonObj[entryKey.toString()] = entryValue.toJson(options);
            }
            break;
          case "enum":
            const enumType = field.V.T;
            for (const [entryKey, entryValue] of Object.entries(value)) {
              assert(entryValue === void 0 || typeof entryValue == "number");
              const val = writeEnum2(enumType, entryValue, true, options.enumAsInteger);
              assert(val !== void 0);
              jsonObj[entryKey.toString()] = val;
            }
            break;
        }
        return options.emitDefaultValues || Object.keys(jsonObj).length > 0 ? jsonObj : void 0;
      } else if (field.repeated) {
        const jsonArr = [];
        switch (field.kind) {
          case "scalar":
            for (let i = 0; i < value.length; i++) {
              jsonArr.push(writeScalar3(field.T, value[i], true));
            }
            break;
          case "enum":
            for (let i = 0; i < value.length; i++) {
              jsonArr.push(writeEnum2(field.T, value[i], true, options.enumAsInteger));
            }
            break;
          case "message":
            for (let i = 0; i < value.length; i++) {
              jsonArr.push(wrapField(field.T, value[i]).toJson(options));
            }
            break;
        }
        return options.emitDefaultValues || jsonArr.length > 0 ? jsonArr : void 0;
      } else {
        switch (field.kind) {
          case "scalar":
            return writeScalar3(field.T, value, !!field.oneof || field.opt || options.emitDefaultValues);
          case "enum":
            return writeEnum2(field.T, value, !!field.oneof || field.opt || options.emitDefaultValues, options.enumAsInteger);
          case "message":
            return value !== void 0 ? wrapField(field.T, value).toJson(options) : void 0;
        }
      }
    };
  });
}

// node_modules/@bufbuild/protobuf/dist/esm/private/util-common.js
function makeUtilCommon() {
  return {
    setEnumType,
    initPartial(source, target) {
      if (source === void 0) {
        return;
      }
      const type = target.getType();
      for (const member of type.fields.byMember()) {
        const localName = member.localName, t = target, s = source;
        if (s[localName] === void 0) {
          continue;
        }
        switch (member.kind) {
          case "oneof":
            const sk = s[localName].case;
            if (sk === void 0) {
              continue;
            }
            const sourceField = member.findField(sk);
            let val = s[localName].value;
            if (sourceField && sourceField.kind == "message" && !(val instanceof sourceField.T)) {
              val = new sourceField.T(val);
            }
            t[localName] = { case: sk, value: val };
            break;
          case "scalar":
          case "enum":
            t[localName] = s[localName];
            break;
          case "map":
            switch (member.V.kind) {
              case "scalar":
              case "enum":
                Object.assign(t[localName], s[localName]);
                break;
              case "message":
                const messageType = member.V.T;
                for (const k of Object.keys(s[localName])) {
                  let val2 = s[localName][k];
                  if (!messageType.fieldWrapper) {
                    val2 = new messageType(val2);
                  }
                  t[localName][k] = val2;
                }
                break;
            }
            break;
          case "message":
            const mt = member.T;
            if (member.repeated) {
              t[localName] = s[localName].map((val2) => val2 instanceof mt ? val2 : new mt(val2));
            } else if (s[localName] !== void 0) {
              const val2 = s[localName];
              if (mt.fieldWrapper) {
                t[localName] = val2;
              } else {
                t[localName] = val2 instanceof mt ? val2 : new mt(val2);
              }
            }
            break;
        }
      }
    },
    equals(type, a, b) {
      if (a === b) {
        return true;
      }
      if (!a || !b) {
        return false;
      }
      return type.fields.byMember().every((m) => {
        const va = a[m.localName];
        const vb = b[m.localName];
        if (m.repeated) {
          if (va.length !== vb.length) {
            return false;
          }
          switch (m.kind) {
            case "message":
              return va.every((a2, i) => m.T.equals(a2, vb[i]));
            case "scalar":
              return va.every((a2, i) => scalarEquals(m.T, a2, vb[i]));
            case "enum":
              return va.every((a2, i) => scalarEquals(ScalarType.INT32, a2, vb[i]));
          }
          throw new Error(`repeated cannot contain ${m.kind}`);
        }
        switch (m.kind) {
          case "message":
            return m.T.equals(va, vb);
          case "enum":
            return scalarEquals(ScalarType.INT32, va, vb);
          case "scalar":
            return scalarEquals(m.T, va, vb);
          case "oneof":
            if (va.case !== vb.case) {
              return false;
            }
            const s = m.findField(va.case);
            if (s === void 0) {
              return true;
            }
            switch (s.kind) {
              case "message":
                return s.T.equals(va.value, vb.value);
              case "enum":
                return scalarEquals(ScalarType.INT32, va.value, vb.value);
              case "scalar":
                return scalarEquals(s.T, va.value, vb.value);
            }
            throw new Error(`oneof cannot contain ${s.kind}`);
          case "map":
            const keys = Object.keys(va).concat(Object.keys(vb));
            switch (m.V.kind) {
              case "message":
                const messageType = m.V.T;
                return keys.every((k) => messageType.equals(va[k], vb[k]));
              case "enum":
                return keys.every((k) => scalarEquals(ScalarType.INT32, va[k], vb[k]));
              case "scalar":
                const scalarType = m.V.T;
                return keys.every((k) => scalarEquals(scalarType, va[k], vb[k]));
            }
            break;
        }
      });
    },
    clone(message) {
      const type = message.getType(), target = new type(), any = target;
      for (const member of type.fields.byMember()) {
        const source = message[member.localName];
        let copy;
        if (member.repeated) {
          copy = source.map((e) => cloneSingularField(member, e));
        } else if (member.kind == "map") {
          copy = any[member.localName];
          for (const [key, v] of Object.entries(source)) {
            copy[key] = cloneSingularField(member.V, v);
          }
        } else if (member.kind == "oneof") {
          const f = member.findField(source.case);
          copy = f ? { case: source.case, value: cloneSingularField(f, source.value) } : { case: void 0 };
        } else {
          copy = cloneSingularField(member, source);
        }
        any[member.localName] = copy;
      }
      return target;
    }
  };
}
function cloneSingularField(field, value) {
  if (value === void 0) {
    return value;
  }
  if (value instanceof Message) {
    return value.clone();
  }
  if (value instanceof Uint8Array) {
    const c = new Uint8Array(value.byteLength);
    c.set(value);
    return c;
  }
  return value;
}

// node_modules/@bufbuild/protobuf/dist/esm/private/field-list.js
var InternalFieldList = class {
  constructor(fields, normalizer) {
    this._fields = fields;
    this._normalizer = normalizer;
  }
  findJsonName(jsonName) {
    if (!this.jsonNames) {
      const t = {};
      for (const f of this.list()) {
        t[f.jsonName] = t[f.name] = f;
      }
      this.jsonNames = t;
    }
    return this.jsonNames[jsonName];
  }
  find(fieldNo) {
    if (!this.numbers) {
      const t = {};
      for (const f of this.list()) {
        t[f.no] = f;
      }
      this.numbers = t;
    }
    return this.numbers[fieldNo];
  }
  list() {
    if (!this.all) {
      this.all = this._normalizer(this._fields);
    }
    return this.all;
  }
  byNumber() {
    if (!this.numbersAsc) {
      this.numbersAsc = this.list().concat().sort((a, b) => a.no - b.no);
    }
    return this.numbersAsc;
  }
  byMember() {
    if (!this.members) {
      this.members = [];
      const a = this.members;
      let o;
      for (const f of this.list()) {
        if (f.oneof) {
          if (f.oneof !== o) {
            o = f.oneof;
            a.push(o);
          }
        } else {
          a.push(f);
        }
      }
    }
    return this.members;
  }
};

// node_modules/@bufbuild/protobuf/dist/esm/private/names.js
function localFieldName(protoName, inOneof) {
  const name = protoCamelCase(protoName);
  if (inOneof) {
    return name;
  }
  return safeObjectProperty(safeMessageProperty(name));
}
function localOneofName(protoName) {
  return localFieldName(protoName, false);
}
var fieldJsonName = protoCamelCase;
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
var reservedMessageProperties = /* @__PURE__ */ new Set([
  // names reserved by the runtime
  "getType",
  "clone",
  "equals",
  "fromBinary",
  "fromJson",
  "fromJsonString",
  "toBinary",
  "toJson",
  "toJsonString",
  // names reserved by the runtime for the future
  "toObject"
]);
var fallback = (name) => `${name}$`;
var safeMessageProperty = (name) => {
  if (reservedMessageProperties.has(name)) {
    return fallback(name);
  }
  return name;
};
var safeObjectProperty = (name) => {
  if (reservedObjectProperties.has(name)) {
    return fallback(name);
  }
  return name;
};

// node_modules/@bufbuild/protobuf/dist/esm/private/field.js
var InternalOneofInfo = class {
  constructor(name) {
    this.kind = "oneof";
    this.repeated = false;
    this.packed = false;
    this.opt = false;
    this.default = void 0;
    this.fields = [];
    this.name = name;
    this.localName = localOneofName(name);
  }
  addField(field) {
    assert(field.oneof === this, `field ${field.name} not one of ${this.name}`);
    this.fields.push(field);
  }
  findField(localName) {
    if (!this._lookup) {
      this._lookup = /* @__PURE__ */ Object.create(null);
      for (let i = 0; i < this.fields.length; i++) {
        this._lookup[this.fields[i].localName] = this.fields[i];
      }
    }
    return this._lookup[localName];
  }
};

// node_modules/@bufbuild/protobuf/dist/esm/proto3.js
var proto3 = makeProtoRuntime("proto3", makeJsonFormatProto3(), makeBinaryFormatProto3(), Object.assign(Object.assign({}, makeUtilCommon()), {
  newFieldList(fields) {
    return new InternalFieldList(fields, normalizeFieldInfosProto3);
  },
  initFields(target) {
    for (const member of target.getType().fields.byMember()) {
      if (member.opt) {
        continue;
      }
      const name = member.localName, t = target;
      if (member.repeated) {
        t[name] = [];
        continue;
      }
      switch (member.kind) {
        case "oneof":
          t[name] = { case: void 0 };
          break;
        case "enum":
          t[name] = 0;
          break;
        case "map":
          t[name] = {};
          break;
        case "scalar":
          t[name] = scalarDefaultValue(member.T);
          break;
        case "message":
          break;
      }
    }
  }
}));
function normalizeFieldInfosProto3(fieldInfos) {
  var _a, _b, _c;
  const r = [];
  let o;
  for (const field of typeof fieldInfos == "function" ? fieldInfos() : fieldInfos) {
    const f = field;
    f.localName = localFieldName(field.name, field.oneof !== void 0);
    f.jsonName = (_a = field.jsonName) !== null && _a !== void 0 ? _a : fieldJsonName(field.name);
    f.repeated = (_b = field.repeated) !== null && _b !== void 0 ? _b : false;
    f.packed = (_c = field.packed) !== null && _c !== void 0 ? _c : field.kind == "enum" || field.kind == "scalar" && field.T != ScalarType.BYTES && field.T != ScalarType.STRING;
    if (field.oneof !== void 0) {
      const ooname = typeof field.oneof == "string" ? field.oneof : field.oneof.name;
      if (!o || o.name != ooname) {
        o = new InternalOneofInfo(ooname);
      }
      f.oneof = o;
      o.addField(f);
    }
    r.push(f);
  }
  return r;
}

// node_modules/@bufbuild/protobuf/dist/esm/google/protobuf/timestamp_pb.js
var Timestamp = class _Timestamp extends Message {
  constructor(data) {
    super();
    this.seconds = protoInt64.zero;
    this.nanos = 0;
    proto3.util.initPartial(data, this);
  }
  fromJson(json, options) {
    if (typeof json !== "string") {
      throw new Error(`cannot decode google.protobuf.Timestamp from JSON: ${proto3.json.debug(json)}`);
    }
    const matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:Z|\.([0-9]{3,9})Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
    if (!matches) {
      throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
    }
    const ms = Date.parse(matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
    if (Number.isNaN(ms)) {
      throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
    }
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) {
      throw new Error(`cannot decode message google.protobuf.Timestamp from JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    this.seconds = protoInt64.parse(ms / 1e3);
    this.nanos = 0;
    if (matches[7]) {
      this.nanos = parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) - 1e9;
    }
    return this;
  }
  toJson(options) {
    const ms = Number(this.seconds) * 1e3;
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) {
      throw new Error(`cannot encode google.protobuf.Timestamp to JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    if (this.nanos < 0) {
      throw new Error(`cannot encode google.protobuf.Timestamp to JSON: nanos must not be negative`);
    }
    let z = "Z";
    if (this.nanos > 0) {
      const nanosStr = (this.nanos + 1e9).toString().substring(1);
      if (nanosStr.substring(3) === "000000") {
        z = "." + nanosStr.substring(0, 3) + "Z";
      } else if (nanosStr.substring(6) === "000") {
        z = "." + nanosStr.substring(0, 6) + "Z";
      } else {
        z = "." + nanosStr + "Z";
      }
    }
    return new Date(ms).toISOString().replace(".000Z", z);
  }
  toDate() {
    return new Date(Number(this.seconds) * 1e3 + Math.ceil(this.nanos / 1e6));
  }
  static now() {
    return _Timestamp.fromDate(/* @__PURE__ */ new Date());
  }
  static fromDate(date) {
    const ms = date.getTime();
    return new _Timestamp({
      seconds: protoInt64.parse(Math.floor(ms / 1e3)),
      nanos: ms % 1e3 * 1e6
    });
  }
  static fromBinary(bytes, options) {
    return new _Timestamp().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Timestamp().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Timestamp().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Timestamp, a, b);
  }
};
Timestamp.runtime = proto3;
Timestamp.typeName = "google.protobuf.Timestamp";
Timestamp.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "seconds",
    kind: "scalar",
    T: 3
    /* ScalarType.INT64 */
  },
  {
    no: 2,
    name: "nanos",
    kind: "scalar",
    T: 5
    /* ScalarType.INT32 */
  }
]);

// pb/sf/ethereum/type/v2/type_pb.ts
var TransactionTraceStatus = /* @__PURE__ */ ((TransactionTraceStatus2) => {
  TransactionTraceStatus2[TransactionTraceStatus2["UNKNOWN"] = 0] = "UNKNOWN";
  TransactionTraceStatus2[TransactionTraceStatus2["SUCCEEDED"] = 1] = "SUCCEEDED";
  TransactionTraceStatus2[TransactionTraceStatus2["FAILED"] = 2] = "FAILED";
  TransactionTraceStatus2[TransactionTraceStatus2["REVERTED"] = 3] = "REVERTED";
  return TransactionTraceStatus2;
})(TransactionTraceStatus || {});
proto3.util.setEnumType(TransactionTraceStatus, "sf.ethereum.type.v2.TransactionTraceStatus", [
  { no: 0, name: "UNKNOWN" },
  { no: 1, name: "SUCCEEDED" },
  { no: 2, name: "FAILED" },
  { no: 3, name: "REVERTED" }
]);
var CallType = /* @__PURE__ */ ((CallType2) => {
  CallType2[CallType2["UNSPECIFIED"] = 0] = "UNSPECIFIED";
  CallType2[CallType2["CALL"] = 1] = "CALL";
  CallType2[CallType2["CALLCODE"] = 2] = "CALLCODE";
  CallType2[CallType2["DELEGATE"] = 3] = "DELEGATE";
  CallType2[CallType2["STATIC"] = 4] = "STATIC";
  CallType2[CallType2["CREATE"] = 5] = "CREATE";
  return CallType2;
})(CallType || {});
proto3.util.setEnumType(CallType, "sf.ethereum.type.v2.CallType", [
  { no: 0, name: "UNSPECIFIED" },
  { no: 1, name: "CALL" },
  { no: 2, name: "CALLCODE" },
  { no: 3, name: "DELEGATE" },
  { no: 4, name: "STATIC" },
  { no: 5, name: "CREATE" }
]);
var _Block = class _Block extends Message {
  constructor(data) {
    super();
    /**
     * Hash is the block's hash.
     *
     * @generated from field: bytes hash = 2;
     */
    this.hash = new Uint8Array(0);
    /**
     * Number is the block's height at which this block was mined.
     *
     * @generated from field: uint64 number = 3;
     */
    this.number = protoInt64.zero;
    /**
     * Size is the size in bytes of the RLP encoding of the block according to Ethereum
     * rules.
     *
     * @generated from field: uint64 size = 4;
     */
    this.size = protoInt64.zero;
    /**
     * Uncles represents block produced with a valid solution but were not actually choosen
     * as the canonical block for the given height so they are mostly "forked" blocks.
     *
     * If the Block has been produced using the Proof of Stake consensus algorithm, this
     * field will actually be always empty.
     *
     * @generated from field: repeated sf.ethereum.type.v2.BlockHeader uncles = 6;
     */
    this.uncles = [];
    /**
     * TransactionTraces hold the execute trace of all the transactions that were executed
     * in this block. In in there that you will find most of the Ethereum data model.
     *
     * @generated from field: repeated sf.ethereum.type.v2.TransactionTrace transaction_traces = 10;
     */
    this.transactionTraces = [];
    /**
     * BalanceChanges here is the array of ETH transfer that happened at the block level
     * outside of the normal transaction flow of a block. The best example of this is mining
     * reward for the block mined, the transfer of ETH to the miner happens outside the normal
     * transaction flow of the chain and is recorded as a `BalanceChange` here since we cannot
     * attached it to any transaction.
     *
     * @generated from field: repeated sf.ethereum.type.v2.BalanceChange balance_changes = 11;
     */
    this.balanceChanges = [];
    /**
     * CodeChanges here is the array of smart code change that happened that happened at the block level
     * outside of the normal transaction flow of a block. Some Ethereum's fork like BSC and Polygon
     * has some capabilities to upgrade internal smart contracts used usually to track the validator
     * list.
     *
     * On hard fork, some procedure runs to upgrade the smart contract code to a new version. In those
     * network, a `CodeChange` for each modified smart contract on upgrade would be present here. Note
     * that this happen rarely, so the vast majority of block will have an empty list here.
     *
     * @generated from field: repeated sf.ethereum.type.v2.CodeChange code_changes = 20;
     */
    this.codeChanges = [];
    /**
     * Ver represents that data model version of the block, it is used internally by Firehose on Ethereum
     * as a validation that we are reading the correct version.
     *
     * @generated from field: int32 ver = 1;
     */
    this.ver = 0;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Block().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Block().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Block().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Block, a, b);
  }
};
_Block.runtime = proto3;
_Block.typeName = "sf.ethereum.type.v2.Block";
_Block.fields = proto3.util.newFieldList(() => [
  {
    no: 2,
    name: "hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 3,
    name: "number",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 4,
    name: "size",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 5, name: "header", kind: "message", T: BlockHeader },
  { no: 6, name: "uncles", kind: "message", T: BlockHeader, repeated: true },
  { no: 10, name: "transaction_traces", kind: "message", T: TransactionTrace, repeated: true },
  { no: 11, name: "balance_changes", kind: "message", T: BalanceChange, repeated: true },
  { no: 20, name: "code_changes", kind: "message", T: CodeChange, repeated: true },
  {
    no: 1,
    name: "ver",
    kind: "scalar",
    T: 5
    /* ScalarType.INT32 */
  }
]);
var Block = _Block;
var _BlockHeader = class _BlockHeader extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes parent_hash = 1;
     */
    this.parentHash = new Uint8Array(0);
    /**
     * Uncle hash of the block, some reference it as `sha3Uncles`, but `sha3`` is badly worded, so we prefer `uncle_hash`, also
     * referred as `ommers` in EIP specification.
     *
     * If the Block containing this `BlockHeader` has been produced using the Proof of Stake
     * consensus algorithm, this field will actually be constant and set to `0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347`.
     *
     * @generated from field: bytes uncle_hash = 2;
     */
    this.uncleHash = new Uint8Array(0);
    /**
     * @generated from field: bytes coinbase = 3;
     */
    this.coinbase = new Uint8Array(0);
    /**
     * @generated from field: bytes state_root = 4;
     */
    this.stateRoot = new Uint8Array(0);
    /**
     * @generated from field: bytes transactions_root = 5;
     */
    this.transactionsRoot = new Uint8Array(0);
    /**
     * @generated from field: bytes receipt_root = 6;
     */
    this.receiptRoot = new Uint8Array(0);
    /**
     * @generated from field: bytes logs_bloom = 7;
     */
    this.logsBloom = new Uint8Array(0);
    /**
     * @generated from field: uint64 number = 9;
     */
    this.number = protoInt64.zero;
    /**
     * @generated from field: uint64 gas_limit = 10;
     */
    this.gasLimit = protoInt64.zero;
    /**
     * @generated from field: uint64 gas_used = 11;
     */
    this.gasUsed = protoInt64.zero;
    /**
     * ExtraData is free-form bytes included in the block by the "miner". While on Yellow paper of
     * Ethereum this value is maxed to 32 bytes, other consensus algorithm like Clique and some other
     * forks are using bigger values to carry special consensus data.
     *
     * If the Block containing this `BlockHeader` has been produced using the Proof of Stake
     * consensus algorithm, this field is strictly enforced to be <= 32 bytes.
     *
     * @generated from field: bytes extra_data = 13;
     */
    this.extraData = new Uint8Array(0);
    /**
     * MixHash is used to prove, when combined with the `nonce` that sufficient amount of computation has been
     * achieved and that the solution found is valid.
     *
     * @generated from field: bytes mix_hash = 14;
     */
    this.mixHash = new Uint8Array(0);
    /**
     * Nonce is used to prove, when combined with the `mix_hash` that sufficient amount of computation has been
     * achieved and that the solution found is valid.
     *
     * If the Block containing this `BlockHeader` has been produced using the Proof of Stake
     * consensus algorithm, this field will actually be constant and set to `0`.
     *
     * @generated from field: uint64 nonce = 15;
     */
    this.nonce = protoInt64.zero;
    /**
     * Hash is the hash of the block which is actually the computation:
     *
     *  Keccak256(rlp([
     *    parent_hash,
     *    uncle_hash,
     *    coinbase,
     *    state_root,
     *    transactions_root,
     *    receipt_root,
     *    logs_bloom,
     *    difficulty,
     *    number,
     *    gas_limit,
     *    gas_used,
     *    timestamp,
     *    extra_data,
     *    mix_hash,
     *    nonce,
     *    base_fee_per_gas (to be included, only if London Fork is active)
     *    withdrawals_root (to be included, only if Shangai Fork is active)
     *  ]))
     *
     *
     * @generated from field: bytes hash = 16;
     */
    this.hash = new Uint8Array(0);
    /**
     * Withdrawals root hash according to EIP-4895 (e.g. Shangai Fork) rules, only set if Shangai is present/active on the chain.
     *
     * @generated from field: bytes withdrawals_root = 19;
     */
    this.withdrawalsRoot = new Uint8Array(0);
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _BlockHeader().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _BlockHeader().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _BlockHeader().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_BlockHeader, a, b);
  }
};
_BlockHeader.runtime = proto3;
_BlockHeader.typeName = "sf.ethereum.type.v2.BlockHeader";
_BlockHeader.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "parent_hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "uncle_hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 3,
    name: "coinbase",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 4,
    name: "state_root",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 5,
    name: "transactions_root",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 6,
    name: "receipt_root",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 7,
    name: "logs_bloom",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 8, name: "difficulty", kind: "message", T: BigInt2 },
  { no: 17, name: "total_difficulty", kind: "message", T: BigInt2 },
  {
    no: 9,
    name: "number",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 10,
    name: "gas_limit",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 11,
    name: "gas_used",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 12, name: "timestamp", kind: "message", T: Timestamp },
  {
    no: 13,
    name: "extra_data",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 14,
    name: "mix_hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 15,
    name: "nonce",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 16,
    name: "hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 18, name: "base_fee_per_gas", kind: "message", T: BigInt2 },
  {
    no: 19,
    name: "withdrawals_root",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 20, name: "tx_dependency", kind: "message", T: Uint64NestedArray }
]);
var BlockHeader = _BlockHeader;
var _Uint64NestedArray = class _Uint64NestedArray extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: repeated sf.ethereum.type.v2.Uint64Array val = 1;
     */
    this.val = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Uint64NestedArray().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Uint64NestedArray().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Uint64NestedArray().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Uint64NestedArray, a, b);
  }
};
_Uint64NestedArray.runtime = proto3;
_Uint64NestedArray.typeName = "sf.ethereum.type.v2.Uint64NestedArray";
_Uint64NestedArray.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "val", kind: "message", T: Uint64Array, repeated: true }
]);
var Uint64NestedArray = _Uint64NestedArray;
var _Uint64Array = class _Uint64Array extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: repeated uint64 val = 1;
     */
    this.val = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Uint64Array().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Uint64Array().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Uint64Array().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Uint64Array, a, b);
  }
};
_Uint64Array.runtime = proto3;
_Uint64Array.typeName = "sf.ethereum.type.v2.Uint64Array";
_Uint64Array.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "val", kind: "scalar", T: 4, repeated: true }
]);
var Uint64Array = _Uint64Array;
var _BigInt = class _BigInt extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes bytes = 1;
     */
    this.bytes = new Uint8Array(0);
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _BigInt().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _BigInt().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _BigInt().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_BigInt, a, b);
  }
};
_BigInt.runtime = proto3;
_BigInt.typeName = "sf.ethereum.type.v2.BigInt";
_BigInt.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "bytes",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  }
]);
var BigInt2 = _BigInt;
var _TransactionTrace = class _TransactionTrace extends Message {
  constructor(data) {
    super();
    /**
     * consensus
     *
     * @generated from field: bytes to = 1;
     */
    this.to = new Uint8Array(0);
    /**
     * @generated from field: uint64 nonce = 2;
     */
    this.nonce = protoInt64.zero;
    /**
     * GasLimit is the maximum of gas unit the sender of the transaction is willing to consume when perform the EVM
     * execution of the whole transaction
     *
     * @generated from field: uint64 gas_limit = 4;
     */
    this.gasLimit = protoInt64.zero;
    /**
     * Input data the transaction will receive for execution of EVM.
     *
     * @generated from field: bytes input = 6;
     */
    this.input = new Uint8Array(0);
    /**
     * V is the recovery ID value for the signature Y point.
     *
     * @generated from field: bytes v = 7;
     */
    this.v = new Uint8Array(0);
    /**
     * R is the signature's X point on the elliptic curve (32 bytes).
     *
     * @generated from field: bytes r = 8;
     */
    this.r = new Uint8Array(0);
    /**
     * S is the signature's Y point on the elliptic curve (32 bytes).
     *
     * @generated from field: bytes s = 9;
     */
    this.s = new Uint8Array(0);
    /**
     * GasUsed is the total amount of gas unit used for the whole execution of the transaction.
     *
     * @generated from field: uint64 gas_used = 10;
     */
    this.gasUsed = protoInt64.zero;
    /**
     * Type represents the Ethereum transaction type, available only since EIP-2718 & EIP-2930 activation which happened on Berlin fork.
     * The value is always set even for transaction before Berlin fork because those before the fork are still legacy transactions.
     *
     * @generated from field: sf.ethereum.type.v2.TransactionTrace.Type type = 12;
     */
    this.type = 0 /* TRX_TYPE_LEGACY */;
    /**
     * AcccessList represents the storage access this transaction has agreed to do in which case those storage
     * access cost less gas unit per access.
     *
     * This will is populated only if `TransactionTrace.Type == TRX_TYPE_ACCESS_LIST || TRX_TYPE_DYNAMIC_FEE` which
     * is possible only if Berlin (TRX_TYPE_ACCESS_LIST) nor London (TRX_TYPE_DYNAMIC_FEE) fork are active on the chain.
     *
     * @generated from field: repeated sf.ethereum.type.v2.AccessTuple access_list = 14;
     */
    this.accessList = [];
    /**
     * meta
     *
     * @generated from field: uint32 index = 20;
     */
    this.index = 0;
    /**
     * @generated from field: bytes hash = 21;
     */
    this.hash = new Uint8Array(0);
    /**
     * @generated from field: bytes from = 22;
     */
    this.from = new Uint8Array(0);
    /**
     * @generated from field: bytes return_data = 23;
     */
    this.returnData = new Uint8Array(0);
    /**
     * @generated from field: bytes public_key = 24;
     */
    this.publicKey = new Uint8Array(0);
    /**
     * @generated from field: uint64 begin_ordinal = 25;
     */
    this.beginOrdinal = protoInt64.zero;
    /**
     * @generated from field: uint64 end_ordinal = 26;
     */
    this.endOrdinal = protoInt64.zero;
    /**
     * TransactionTraceStatus is the status of the transaction execution and will let you know if the transaction
     * was successful or not.
     *
     * A successful transaction has been recorded to the blockchain's state for calls in it that were successful.
     * This means it's possible only a subset of the calls were properly recorded, refer to [calls[].state_reverted] field
     * to determine which calls were reverted.
     *
     * A quirks of the Ethereum protocol is that a transaction `FAILED` or `REVERTED` still affects the blockchain's
     * state for **some** of the state changes. Indeed, in those cases, the transactions fees are still paid to the miner
     * which means there is a balance change for the transaction's emitter (e.g. `from`) to pay the gas fees, an optional
     * balance change for gas refunded to the transaction's emitter (e.g. `from`) and a balance change for the miner who
     * received the transaction fees. There is also a nonce change for the transaction's emitter (e.g. `from`).
     *
     * This means that to properly record the state changes for a transaction, you need to conditionally procees the
     * transaction's status.
     *
     * For a `SUCCEEDED` transaction, you iterate over the `calls` array and record the state changes for each call for
     * which `state_reverted == false` (if a transaction succeeded, the call at #0 will always `state_reverted == false`
     * because it aligns with the transaction).
     *
     * For a `FAILED` or `REVERTED` transaction, you iterate over the root call (e.g. at #0, will always exist) for
     * balance changes you process those where `reason` is either `REASON_GAS_BUY`, `REASON_GAS_REFUND` or
     * `REASON_REWARD_TRANSACTION_FEE` and for nonce change, still on the root call, you pick the nonce change which the
     * smallest ordinal (if more than one).
     *
     * @generated from field: sf.ethereum.type.v2.TransactionTraceStatus status = 30;
     */
    this.status = 0 /* UNKNOWN */;
    /**
     * @generated from field: repeated sf.ethereum.type.v2.Call calls = 32;
     */
    this.calls = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _TransactionTrace().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _TransactionTrace().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _TransactionTrace().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_TransactionTrace, a, b);
  }
};
_TransactionTrace.runtime = proto3;
_TransactionTrace.typeName = "sf.ethereum.type.v2.TransactionTrace";
_TransactionTrace.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "to",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "nonce",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 3, name: "gas_price", kind: "message", T: BigInt2 },
  {
    no: 4,
    name: "gas_limit",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 5, name: "value", kind: "message", T: BigInt2 },
  {
    no: 6,
    name: "input",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 7,
    name: "v",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 8,
    name: "r",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 9,
    name: "s",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 10,
    name: "gas_used",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 12, name: "type", kind: "enum", T: proto3.getEnumType(TransactionTrace_Type) },
  { no: 14, name: "access_list", kind: "message", T: AccessTuple, repeated: true },
  { no: 11, name: "max_fee_per_gas", kind: "message", T: BigInt2 },
  { no: 13, name: "max_priority_fee_per_gas", kind: "message", T: BigInt2 },
  {
    no: 20,
    name: "index",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  {
    no: 21,
    name: "hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 22,
    name: "from",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 23,
    name: "return_data",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 24,
    name: "public_key",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 25,
    name: "begin_ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 26,
    name: "end_ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 30, name: "status", kind: "enum", T: proto3.getEnumType(TransactionTraceStatus) },
  { no: 31, name: "receipt", kind: "message", T: TransactionReceipt },
  { no: 32, name: "calls", kind: "message", T: Call, repeated: true }
]);
var TransactionTrace = _TransactionTrace;
var TransactionTrace_Type = /* @__PURE__ */ ((TransactionTrace_Type2) => {
  TransactionTrace_Type2[TransactionTrace_Type2["TRX_TYPE_LEGACY"] = 0] = "TRX_TYPE_LEGACY";
  TransactionTrace_Type2[TransactionTrace_Type2["TRX_TYPE_ACCESS_LIST"] = 1] = "TRX_TYPE_ACCESS_LIST";
  TransactionTrace_Type2[TransactionTrace_Type2["TRX_TYPE_DYNAMIC_FEE"] = 2] = "TRX_TYPE_DYNAMIC_FEE";
  return TransactionTrace_Type2;
})(TransactionTrace_Type || {});
proto3.util.setEnumType(TransactionTrace_Type, "sf.ethereum.type.v2.TransactionTrace.Type", [
  { no: 0, name: "TRX_TYPE_LEGACY" },
  { no: 1, name: "TRX_TYPE_ACCESS_LIST" },
  { no: 2, name: "TRX_TYPE_DYNAMIC_FEE" }
]);
var _AccessTuple = class _AccessTuple extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: repeated bytes storage_keys = 2;
     */
    this.storageKeys = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _AccessTuple().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _AccessTuple().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _AccessTuple().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_AccessTuple, a, b);
  }
};
_AccessTuple.runtime = proto3;
_AccessTuple.typeName = "sf.ethereum.type.v2.AccessTuple";
_AccessTuple.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 2, name: "storage_keys", kind: "scalar", T: 12, repeated: true }
]);
var AccessTuple = _AccessTuple;
var _TransactionReceipt = class _TransactionReceipt extends Message {
  constructor(data) {
    super();
    /**
     * State root is an intermediate state_root hash, computed in-between transactions to make
     * **sure** you could build a proof and point to state in the middle of a block. Geth client
     * uses `PostState + root + PostStateOrStatus`` while Parity used `status_code, root...`` this piles
     * hardforks, see (read the EIPs first):
     * - https://github.com/ethereum/EIPs/blob/master/EIPS/eip-658.md
     *
     * Moreover, the notion of `Outcome`` in parity, which segregates the two concepts, which are
     * stored in the same field `status_code`` can be computed based on such a hack of the `state_root`
     * field, following `EIP-658`.
     *
     * Before Byzantinium hard fork, this field is always empty.
     *
     * @generated from field: bytes state_root = 1;
     */
    this.stateRoot = new Uint8Array(0);
    /**
     * @generated from field: uint64 cumulative_gas_used = 2;
     */
    this.cumulativeGasUsed = protoInt64.zero;
    /**
     * @generated from field: bytes logs_bloom = 3;
     */
    this.logsBloom = new Uint8Array(0);
    /**
     * @generated from field: repeated sf.ethereum.type.v2.Log logs = 4;
     */
    this.logs = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _TransactionReceipt().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _TransactionReceipt().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _TransactionReceipt().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_TransactionReceipt, a, b);
  }
};
_TransactionReceipt.runtime = proto3;
_TransactionReceipt.typeName = "sf.ethereum.type.v2.TransactionReceipt";
_TransactionReceipt.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "state_root",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "cumulative_gas_used",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 3,
    name: "logs_bloom",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 4, name: "logs", kind: "message", T: Log, repeated: true }
]);
var TransactionReceipt = _TransactionReceipt;
var _Log = class _Log extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: repeated bytes topics = 2;
     */
    this.topics = [];
    /**
     * @generated from field: bytes data = 3;
     */
    this.data = new Uint8Array(0);
    /**
     * Index is the index of the log relative to the transaction. This index
     * is always populated regardless of the state revertion of the the call
     * that emitted this log.
     *
     * @generated from field: uint32 index = 4;
     */
    this.index = 0;
    /**
     * BlockIndex represents the index of the log relative to the Block.
     *
     * An **important** notice is that this field will be 0 when the call
     * that emitted the log has been reverted by the chain.
     *
     * Currently, there is two locations where a Log can be obtained:
     * - block.transaction_traces[].receipt.logs[]
     * - block.transaction_traces[].calls[].logs[]
     *
     * In the `receipt` case, the logs will be populated only when the call
     * that emitted them has not been reverted by the chain and when in this
     * position, the `blockIndex` is always populated correctly.
     *
     * In the case of `calls` case, for `call` where `stateReverted == true`,
     * the `blockIndex` value will always be 0.
     *
     * @generated from field: uint32 blockIndex = 6;
     */
    this.blockIndex = 0;
    /**
     * @generated from field: uint64 ordinal = 7;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Log().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Log().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Log().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Log, a, b);
  }
};
_Log.runtime = proto3;
_Log.typeName = "sf.ethereum.type.v2.Log";
_Log.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 2, name: "topics", kind: "scalar", T: 12, repeated: true },
  {
    no: 3,
    name: "data",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 4,
    name: "index",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  {
    no: 6,
    name: "blockIndex",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  {
    no: 7,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var Log = _Log;
var _Call = class _Call extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: uint32 index = 1;
     */
    this.index = 0;
    /**
     * @generated from field: uint32 parent_index = 2;
     */
    this.parentIndex = 0;
    /**
     * @generated from field: uint32 depth = 3;
     */
    this.depth = 0;
    /**
     * @generated from field: sf.ethereum.type.v2.CallType call_type = 4;
     */
    this.callType = 0 /* UNSPECIFIED */;
    /**
     * @generated from field: bytes caller = 5;
     */
    this.caller = new Uint8Array(0);
    /**
     * @generated from field: bytes address = 6;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: uint64 gas_limit = 8;
     */
    this.gasLimit = protoInt64.zero;
    /**
     * @generated from field: uint64 gas_consumed = 9;
     */
    this.gasConsumed = protoInt64.zero;
    /**
     * @generated from field: bytes return_data = 13;
     */
    this.returnData = new Uint8Array(0);
    /**
     * @generated from field: bytes input = 14;
     */
    this.input = new Uint8Array(0);
    /**
     * @generated from field: bool executed_code = 15;
     */
    this.executedCode = false;
    /**
     * @generated from field: bool suicide = 16;
     */
    this.suicide = false;
    /**
     * hex representation of the hash -> preimage 
     *
     * @generated from field: map<string, string> keccak_preimages = 20;
     */
    this.keccakPreimages = {};
    /**
     * @generated from field: repeated sf.ethereum.type.v2.StorageChange storage_changes = 21;
     */
    this.storageChanges = [];
    /**
     * @generated from field: repeated sf.ethereum.type.v2.BalanceChange balance_changes = 22;
     */
    this.balanceChanges = [];
    /**
     * @generated from field: repeated sf.ethereum.type.v2.NonceChange nonce_changes = 24;
     */
    this.nonceChanges = [];
    /**
     * @generated from field: repeated sf.ethereum.type.v2.Log logs = 25;
     */
    this.logs = [];
    /**
     * @generated from field: repeated sf.ethereum.type.v2.CodeChange code_changes = 26;
     */
    this.codeChanges = [];
    /**
     * @generated from field: repeated sf.ethereum.type.v2.GasChange gas_changes = 28;
     */
    this.gasChanges = [];
    /**
     * In Ethereum, a call can be either:
     * - Successfull, execution passes without any problem encountered
     * - Failed, execution failed, and remaining gas should be consumed
     * - Reverted, execution failed, but only gas consumed so far is billed, remaining gas is refunded
     *
     * When a call is either `failed` or `reverted`, the `status_failed` field
     * below is set to `true`. If the status is `reverted`, then both `status_failed`
     * and `status_reverted` are going to be set to `true`.
     *
     * @generated from field: bool status_failed = 10;
     */
    this.statusFailed = false;
    /**
     * @generated from field: bool status_reverted = 12;
     */
    this.statusReverted = false;
    /**
     * Populated when a call either failed or reverted, so when `status_failed == true`,
     * see above for details about those flags.
     *
     * @generated from field: string failure_reason = 11;
     */
    this.failureReason = "";
    /**
     * This field represents wheter or not the state changes performed
     * by this call were correctly recorded by the blockchain.
     *
     * On Ethereum, a transaction can record state changes even if some
     * of its inner nested calls failed. This is problematic however since
     * a call will invalidate all its state changes as well as all state
     * changes performed by its child call. This means that even if a call
     * has a status of `SUCCESS`, the chain might have reverted all the state
     * changes it performed.
     *
     * ```text
     *   Trx 1
     *    Call #1 <Failed>
     *      Call #2 <Execution Success>
     *      Call #3 <Execution Success>
     *      |--- Failure here
     *    Call #4
     * ```
     *
     * In the transaction above, while Call #2 and Call #3 would have the
     * status `EXECUTED`.
     *
     * If you check all calls and check only `state_reverted` flag, you might be missing
     * some balance changes and nonce changes. This is because when a full transaction fails
     * in ethereum (e.g. `calls.all(x.state_reverted == true)`), there is still the transaction
     * fee that are recorded to the chain.
     *
     * Refer to [TransactionTrace#status] field for more details about the handling you must
     * perform.
     *
     * @generated from field: bool state_reverted = 30;
     */
    this.stateReverted = false;
    /**
     * @generated from field: uint64 begin_ordinal = 31;
     */
    this.beginOrdinal = protoInt64.zero;
    /**
     * @generated from field: uint64 end_ordinal = 32;
     */
    this.endOrdinal = protoInt64.zero;
    /**
     * @generated from field: repeated sf.ethereum.type.v2.AccountCreation account_creations = 33;
     */
    this.accountCreations = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Call().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Call().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Call().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Call, a, b);
  }
};
_Call.runtime = proto3;
_Call.typeName = "sf.ethereum.type.v2.Call";
_Call.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "index",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  {
    no: 2,
    name: "parent_index",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  {
    no: 3,
    name: "depth",
    kind: "scalar",
    T: 13
    /* ScalarType.UINT32 */
  },
  { no: 4, name: "call_type", kind: "enum", T: proto3.getEnumType(CallType) },
  {
    no: 5,
    name: "caller",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 6,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 7, name: "value", kind: "message", T: BigInt2 },
  {
    no: 8,
    name: "gas_limit",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 9,
    name: "gas_consumed",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 13,
    name: "return_data",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 14,
    name: "input",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 15,
    name: "executed_code",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  },
  {
    no: 16,
    name: "suicide",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  },
  { no: 20, name: "keccak_preimages", kind: "map", K: 9, V: {
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  } },
  { no: 21, name: "storage_changes", kind: "message", T: StorageChange, repeated: true },
  { no: 22, name: "balance_changes", kind: "message", T: BalanceChange, repeated: true },
  { no: 24, name: "nonce_changes", kind: "message", T: NonceChange, repeated: true },
  { no: 25, name: "logs", kind: "message", T: Log, repeated: true },
  { no: 26, name: "code_changes", kind: "message", T: CodeChange, repeated: true },
  { no: 28, name: "gas_changes", kind: "message", T: GasChange, repeated: true },
  {
    no: 10,
    name: "status_failed",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  },
  {
    no: 12,
    name: "status_reverted",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  },
  {
    no: 11,
    name: "failure_reason",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  },
  {
    no: 30,
    name: "state_reverted",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  },
  {
    no: 31,
    name: "begin_ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 32,
    name: "end_ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 33, name: "account_creations", kind: "message", T: AccountCreation, repeated: true }
]);
var Call = _Call;
var _StorageChange = class _StorageChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: bytes key = 2;
     */
    this.key = new Uint8Array(0);
    /**
     * @generated from field: bytes old_value = 3;
     */
    this.oldValue = new Uint8Array(0);
    /**
     * @generated from field: bytes new_value = 4;
     */
    this.newValue = new Uint8Array(0);
    /**
     * @generated from field: uint64 ordinal = 5;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _StorageChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _StorageChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _StorageChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_StorageChange, a, b);
  }
};
_StorageChange.runtime = proto3;
_StorageChange.typeName = "sf.ethereum.type.v2.StorageChange";
_StorageChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "key",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 3,
    name: "old_value",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 4,
    name: "new_value",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 5,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var StorageChange = _StorageChange;
var _BalanceChange = class _BalanceChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: sf.ethereum.type.v2.BalanceChange.Reason reason = 4;
     */
    this.reason = 0 /* UNKNOWN */;
    /**
     * @generated from field: uint64 ordinal = 5;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _BalanceChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _BalanceChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _BalanceChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_BalanceChange, a, b);
  }
};
_BalanceChange.runtime = proto3;
_BalanceChange.typeName = "sf.ethereum.type.v2.BalanceChange";
_BalanceChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  { no: 2, name: "old_value", kind: "message", T: BigInt2 },
  { no: 3, name: "new_value", kind: "message", T: BigInt2 },
  { no: 4, name: "reason", kind: "enum", T: proto3.getEnumType(BalanceChange_Reason) },
  {
    no: 5,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var BalanceChange = _BalanceChange;
var BalanceChange_Reason = /* @__PURE__ */ ((BalanceChange_Reason2) => {
  BalanceChange_Reason2[BalanceChange_Reason2["UNKNOWN"] = 0] = "UNKNOWN";
  BalanceChange_Reason2[BalanceChange_Reason2["REWARD_MINE_UNCLE"] = 1] = "REWARD_MINE_UNCLE";
  BalanceChange_Reason2[BalanceChange_Reason2["REWARD_MINE_BLOCK"] = 2] = "REWARD_MINE_BLOCK";
  BalanceChange_Reason2[BalanceChange_Reason2["DAO_REFUND_CONTRACT"] = 3] = "DAO_REFUND_CONTRACT";
  BalanceChange_Reason2[BalanceChange_Reason2["DAO_ADJUST_BALANCE"] = 4] = "DAO_ADJUST_BALANCE";
  BalanceChange_Reason2[BalanceChange_Reason2["TRANSFER"] = 5] = "TRANSFER";
  BalanceChange_Reason2[BalanceChange_Reason2["GENESIS_BALANCE"] = 6] = "GENESIS_BALANCE";
  BalanceChange_Reason2[BalanceChange_Reason2["GAS_BUY"] = 7] = "GAS_BUY";
  BalanceChange_Reason2[BalanceChange_Reason2["REWARD_TRANSACTION_FEE"] = 8] = "REWARD_TRANSACTION_FEE";
  BalanceChange_Reason2[BalanceChange_Reason2["REWARD_FEE_RESET"] = 14] = "REWARD_FEE_RESET";
  BalanceChange_Reason2[BalanceChange_Reason2["GAS_REFUND"] = 9] = "GAS_REFUND";
  BalanceChange_Reason2[BalanceChange_Reason2["TOUCH_ACCOUNT"] = 10] = "TOUCH_ACCOUNT";
  BalanceChange_Reason2[BalanceChange_Reason2["SUICIDE_REFUND"] = 11] = "SUICIDE_REFUND";
  BalanceChange_Reason2[BalanceChange_Reason2["SUICIDE_WITHDRAW"] = 13] = "SUICIDE_WITHDRAW";
  BalanceChange_Reason2[BalanceChange_Reason2["CALL_BALANCE_OVERRIDE"] = 12] = "CALL_BALANCE_OVERRIDE";
  BalanceChange_Reason2[BalanceChange_Reason2["BURN"] = 15] = "BURN";
  BalanceChange_Reason2[BalanceChange_Reason2["WITHDRAWAL"] = 16] = "WITHDRAWAL";
  return BalanceChange_Reason2;
})(BalanceChange_Reason || {});
proto3.util.setEnumType(BalanceChange_Reason, "sf.ethereum.type.v2.BalanceChange.Reason", [
  { no: 0, name: "REASON_UNKNOWN" },
  { no: 1, name: "REASON_REWARD_MINE_UNCLE" },
  { no: 2, name: "REASON_REWARD_MINE_BLOCK" },
  { no: 3, name: "REASON_DAO_REFUND_CONTRACT" },
  { no: 4, name: "REASON_DAO_ADJUST_BALANCE" },
  { no: 5, name: "REASON_TRANSFER" },
  { no: 6, name: "REASON_GENESIS_BALANCE" },
  { no: 7, name: "REASON_GAS_BUY" },
  { no: 8, name: "REASON_REWARD_TRANSACTION_FEE" },
  { no: 14, name: "REASON_REWARD_FEE_RESET" },
  { no: 9, name: "REASON_GAS_REFUND" },
  { no: 10, name: "REASON_TOUCH_ACCOUNT" },
  { no: 11, name: "REASON_SUICIDE_REFUND" },
  { no: 13, name: "REASON_SUICIDE_WITHDRAW" },
  { no: 12, name: "REASON_CALL_BALANCE_OVERRIDE" },
  { no: 15, name: "REASON_BURN" },
  { no: 16, name: "REASON_WITHDRAWAL" }
]);
var _NonceChange = class _NonceChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: uint64 old_value = 2;
     */
    this.oldValue = protoInt64.zero;
    /**
     * @generated from field: uint64 new_value = 3;
     */
    this.newValue = protoInt64.zero;
    /**
     * @generated from field: uint64 ordinal = 4;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _NonceChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _NonceChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _NonceChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_NonceChange, a, b);
  }
};
_NonceChange.runtime = proto3;
_NonceChange.typeName = "sf.ethereum.type.v2.NonceChange";
_NonceChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "old_value",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 3,
    name: "new_value",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 4,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var NonceChange = _NonceChange;
var _AccountCreation = class _AccountCreation extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes account = 1;
     */
    this.account = new Uint8Array(0);
    /**
     * @generated from field: uint64 ordinal = 2;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _AccountCreation().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _AccountCreation().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _AccountCreation().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_AccountCreation, a, b);
  }
};
_AccountCreation.runtime = proto3;
_AccountCreation.typeName = "sf.ethereum.type.v2.AccountCreation";
_AccountCreation.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "account",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var AccountCreation = _AccountCreation;
var _CodeChange = class _CodeChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes address = 1;
     */
    this.address = new Uint8Array(0);
    /**
     * @generated from field: bytes old_hash = 2;
     */
    this.oldHash = new Uint8Array(0);
    /**
     * @generated from field: bytes old_code = 3;
     */
    this.oldCode = new Uint8Array(0);
    /**
     * @generated from field: bytes new_hash = 4;
     */
    this.newHash = new Uint8Array(0);
    /**
     * @generated from field: bytes new_code = 5;
     */
    this.newCode = new Uint8Array(0);
    /**
     * @generated from field: uint64 ordinal = 6;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _CodeChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _CodeChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _CodeChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_CodeChange, a, b);
  }
};
_CodeChange.runtime = proto3;
_CodeChange.typeName = "sf.ethereum.type.v2.CodeChange";
_CodeChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "address",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "old_hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 3,
    name: "old_code",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 4,
    name: "new_hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 5,
    name: "new_code",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 6,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var CodeChange = _CodeChange;
var _GasChange = class _GasChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: uint64 old_value = 1;
     */
    this.oldValue = protoInt64.zero;
    /**
     * @generated from field: uint64 new_value = 2;
     */
    this.newValue = protoInt64.zero;
    /**
     * @generated from field: sf.ethereum.type.v2.GasChange.Reason reason = 3;
     */
    this.reason = 0 /* UNKNOWN */;
    /**
     * @generated from field: uint64 ordinal = 4;
     */
    this.ordinal = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _GasChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _GasChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _GasChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_GasChange, a, b);
  }
};
_GasChange.runtime = proto3;
_GasChange.typeName = "sf.ethereum.type.v2.GasChange";
_GasChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "old_value",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  {
    no: 2,
    name: "new_value",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 3, name: "reason", kind: "enum", T: proto3.getEnumType(GasChange_Reason) },
  {
    no: 4,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var GasChange = _GasChange;
var GasChange_Reason = /* @__PURE__ */ ((GasChange_Reason2) => {
  GasChange_Reason2[GasChange_Reason2["UNKNOWN"] = 0] = "UNKNOWN";
  GasChange_Reason2[GasChange_Reason2["CALL"] = 1] = "CALL";
  GasChange_Reason2[GasChange_Reason2["CALL_CODE"] = 2] = "CALL_CODE";
  GasChange_Reason2[GasChange_Reason2["CALL_DATA_COPY"] = 3] = "CALL_DATA_COPY";
  GasChange_Reason2[GasChange_Reason2["CODE_COPY"] = 4] = "CODE_COPY";
  GasChange_Reason2[GasChange_Reason2["CODE_STORAGE"] = 5] = "CODE_STORAGE";
  GasChange_Reason2[GasChange_Reason2["CONTRACT_CREATION"] = 6] = "CONTRACT_CREATION";
  GasChange_Reason2[GasChange_Reason2["CONTRACT_CREATION2"] = 7] = "CONTRACT_CREATION2";
  GasChange_Reason2[GasChange_Reason2["DELEGATE_CALL"] = 8] = "DELEGATE_CALL";
  GasChange_Reason2[GasChange_Reason2["EVENT_LOG"] = 9] = "EVENT_LOG";
  GasChange_Reason2[GasChange_Reason2["EXT_CODE_COPY"] = 10] = "EXT_CODE_COPY";
  GasChange_Reason2[GasChange_Reason2["FAILED_EXECUTION"] = 11] = "FAILED_EXECUTION";
  GasChange_Reason2[GasChange_Reason2["INTRINSIC_GAS"] = 12] = "INTRINSIC_GAS";
  GasChange_Reason2[GasChange_Reason2["PRECOMPILED_CONTRACT"] = 13] = "PRECOMPILED_CONTRACT";
  GasChange_Reason2[GasChange_Reason2["REFUND_AFTER_EXECUTION"] = 14] = "REFUND_AFTER_EXECUTION";
  GasChange_Reason2[GasChange_Reason2["RETURN"] = 15] = "RETURN";
  GasChange_Reason2[GasChange_Reason2["RETURN_DATA_COPY"] = 16] = "RETURN_DATA_COPY";
  GasChange_Reason2[GasChange_Reason2["REVERT"] = 17] = "REVERT";
  GasChange_Reason2[GasChange_Reason2["SELF_DESTRUCT"] = 18] = "SELF_DESTRUCT";
  GasChange_Reason2[GasChange_Reason2["STATIC_CALL"] = 19] = "STATIC_CALL";
  GasChange_Reason2[GasChange_Reason2["STATE_COLD_ACCESS"] = 20] = "STATE_COLD_ACCESS";
  return GasChange_Reason2;
})(GasChange_Reason || {});
proto3.util.setEnumType(GasChange_Reason, "sf.ethereum.type.v2.GasChange.Reason", [
  { no: 0, name: "REASON_UNKNOWN" },
  { no: 1, name: "REASON_CALL" },
  { no: 2, name: "REASON_CALL_CODE" },
  { no: 3, name: "REASON_CALL_DATA_COPY" },
  { no: 4, name: "REASON_CODE_COPY" },
  { no: 5, name: "REASON_CODE_STORAGE" },
  { no: 6, name: "REASON_CONTRACT_CREATION" },
  { no: 7, name: "REASON_CONTRACT_CREATION2" },
  { no: 8, name: "REASON_DELEGATE_CALL" },
  { no: 9, name: "REASON_EVENT_LOG" },
  { no: 10, name: "REASON_EXT_CODE_COPY" },
  { no: 11, name: "REASON_FAILED_EXECUTION" },
  { no: 12, name: "REASON_INTRINSIC_GAS" },
  { no: 13, name: "REASON_PRECOMPILED_CONTRACT" },
  { no: 14, name: "REASON_REFUND_AFTER_EXECUTION" },
  { no: 15, name: "REASON_RETURN" },
  { no: 16, name: "REASON_RETURN_DATA_COPY" },
  { no: 17, name: "REASON_REVERT" },
  { no: 18, name: "REASON_SELF_DESTRUCT" },
  { no: 19, name: "REASON_STATIC_CALL" },
  { no: 20, name: "REASON_STATE_COLD_ACCESS" }
]);
var _HeaderOnlyBlock = class _HeaderOnlyBlock extends Message {
  constructor(data) {
    super();
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _HeaderOnlyBlock().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _HeaderOnlyBlock().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _HeaderOnlyBlock().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_HeaderOnlyBlock, a, b);
  }
};
_HeaderOnlyBlock.runtime = proto3;
_HeaderOnlyBlock.typeName = "sf.ethereum.type.v2.HeaderOnlyBlock";
_HeaderOnlyBlock.fields = proto3.util.newFieldList(() => [
  { no: 5, name: "header", kind: "message", T: BlockHeader }
]);
var HeaderOnlyBlock = _HeaderOnlyBlock;
var _BlockWithRefs = class _BlockWithRefs extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: string id = 1;
     */
    this.id = "";
    /**
     * @generated from field: bool irreversible = 4;
     */
    this.irreversible = false;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _BlockWithRefs().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _BlockWithRefs().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _BlockWithRefs().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_BlockWithRefs, a, b);
  }
};
_BlockWithRefs.runtime = proto3;
_BlockWithRefs.typeName = "sf.ethereum.type.v2.BlockWithRefs";
_BlockWithRefs.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "id",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  },
  { no: 2, name: "block", kind: "message", T: Block },
  { no: 3, name: "transaction_trace_refs", kind: "message", T: TransactionRefs },
  {
    no: 4,
    name: "irreversible",
    kind: "scalar",
    T: 8
    /* ScalarType.BOOL */
  }
]);
var BlockWithRefs = _BlockWithRefs;
var _TransactionTraceWithBlockRef = class _TransactionTraceWithBlockRef extends Message {
  constructor(data) {
    super();
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _TransactionTraceWithBlockRef().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _TransactionTraceWithBlockRef().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _TransactionTraceWithBlockRef().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_TransactionTraceWithBlockRef, a, b);
  }
};
_TransactionTraceWithBlockRef.runtime = proto3;
_TransactionTraceWithBlockRef.typeName = "sf.ethereum.type.v2.TransactionTraceWithBlockRef";
_TransactionTraceWithBlockRef.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "trace", kind: "message", T: TransactionTrace },
  { no: 2, name: "block_ref", kind: "message", T: BlockRef }
]);
var TransactionTraceWithBlockRef = _TransactionTraceWithBlockRef;
var _TransactionRefs = class _TransactionRefs extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: repeated bytes hashes = 1;
     */
    this.hashes = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _TransactionRefs().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _TransactionRefs().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _TransactionRefs().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_TransactionRefs, a, b);
  }
};
_TransactionRefs.runtime = proto3;
_TransactionRefs.typeName = "sf.ethereum.type.v2.TransactionRefs";
_TransactionRefs.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "hashes", kind: "scalar", T: 12, repeated: true }
]);
var TransactionRefs = _TransactionRefs;
var _BlockRef = class _BlockRef extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: bytes hash = 1;
     */
    this.hash = new Uint8Array(0);
    /**
     * @generated from field: uint64 number = 2;
     */
    this.number = protoInt64.zero;
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _BlockRef().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _BlockRef().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _BlockRef().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_BlockRef, a, b);
  }
};
_BlockRef.runtime = proto3;
_BlockRef.typeName = "sf.ethereum.type.v2.BlockRef";
_BlockRef.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "hash",
    kind: "scalar",
    T: 12
    /* ScalarType.BYTES */
  },
  {
    no: 2,
    name: "number",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  }
]);
var BlockRef = _BlockRef;

// pb/sf/substreams/sink/database/v1/database_pb.ts
var _DatabaseChanges = class _DatabaseChanges extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: repeated sf.substreams.sink.database.v1.TableChange table_changes = 1;
     */
    this.tableChanges = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _DatabaseChanges().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _DatabaseChanges().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _DatabaseChanges().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_DatabaseChanges, a, b);
  }
};
_DatabaseChanges.runtime = proto3;
_DatabaseChanges.typeName = "sf.substreams.sink.database.v1.DatabaseChanges";
_DatabaseChanges.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "table_changes", kind: "message", T: TableChange, repeated: true }
]);
var DatabaseChanges = _DatabaseChanges;
var _TableChange = class _TableChange extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: string table = 1;
     */
    this.table = "";
    /**
     * @generated from oneof sf.substreams.sink.database.v1.TableChange.primary_key
     */
    this.primaryKey = { case: void 0 };
    /**
     * @generated from field: uint64 ordinal = 3;
     */
    this.ordinal = protoInt64.zero;
    /**
     * @generated from field: sf.substreams.sink.database.v1.TableChange.Operation operation = 4;
     */
    this.operation = 0 /* UNSPECIFIED */;
    /**
     * @generated from field: repeated sf.substreams.sink.database.v1.Field fields = 5;
     */
    this.fields = [];
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _TableChange().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _TableChange().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _TableChange().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_TableChange, a, b);
  }
};
_TableChange.runtime = proto3;
_TableChange.typeName = "sf.substreams.sink.database.v1.TableChange";
_TableChange.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "table",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  },
  { no: 2, name: "pk", kind: "scalar", T: 9, oneof: "primary_key" },
  { no: 6, name: "composite_pk", kind: "message", T: CompositePrimaryKey, oneof: "primary_key" },
  {
    no: 3,
    name: "ordinal",
    kind: "scalar",
    T: 4
    /* ScalarType.UINT64 */
  },
  { no: 4, name: "operation", kind: "enum", T: proto3.getEnumType(TableChange_Operation) },
  { no: 5, name: "fields", kind: "message", T: Field, repeated: true }
]);
var TableChange = _TableChange;
var TableChange_Operation = /* @__PURE__ */ ((TableChange_Operation2) => {
  TableChange_Operation2[TableChange_Operation2["UNSPECIFIED"] = 0] = "UNSPECIFIED";
  TableChange_Operation2[TableChange_Operation2["CREATE"] = 1] = "CREATE";
  TableChange_Operation2[TableChange_Operation2["UPDATE"] = 2] = "UPDATE";
  TableChange_Operation2[TableChange_Operation2["DELETE"] = 3] = "DELETE";
  return TableChange_Operation2;
})(TableChange_Operation || {});
proto3.util.setEnumType(TableChange_Operation, "sf.substreams.sink.database.v1.TableChange.Operation", [
  { no: 0, name: "OPERATION_UNSPECIFIED" },
  { no: 1, name: "OPERATION_CREATE" },
  { no: 2, name: "OPERATION_UPDATE" },
  { no: 3, name: "OPERATION_DELETE" }
]);
var _CompositePrimaryKey = class _CompositePrimaryKey extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: map<string, string> keys = 1;
     */
    this.keys = {};
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _CompositePrimaryKey().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _CompositePrimaryKey().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _CompositePrimaryKey().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_CompositePrimaryKey, a, b);
  }
};
_CompositePrimaryKey.runtime = proto3;
_CompositePrimaryKey.typeName = "sf.substreams.sink.database.v1.CompositePrimaryKey";
_CompositePrimaryKey.fields = proto3.util.newFieldList(() => [
  { no: 1, name: "keys", kind: "map", K: 9, V: {
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  } }
]);
var CompositePrimaryKey = _CompositePrimaryKey;
var _Field = class _Field extends Message {
  constructor(data) {
    super();
    /**
     * @generated from field: string name = 1;
     */
    this.name = "";
    /**
     * @generated from field: string new_value = 2;
     */
    this.newValue = "";
    /**
     * @generated from field: string old_value = 3;
     */
    this.oldValue = "";
    proto3.util.initPartial(data, this);
  }
  static fromBinary(bytes, options) {
    return new _Field().fromBinary(bytes, options);
  }
  static fromJson(jsonValue, options) {
    return new _Field().fromJson(jsonValue, options);
  }
  static fromJsonString(jsonString, options) {
    return new _Field().fromJsonString(jsonString, options);
  }
  static equals(a, b) {
    return proto3.util.equals(_Field, a, b);
  }
};
_Field.runtime = proto3;
_Field.typeName = "sf.substreams.sink.database.v1.Field";
_Field.fields = proto3.util.newFieldList(() => [
  {
    no: 1,
    name: "name",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  },
  {
    no: 2,
    name: "new_value",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  },
  {
    no: 3,
    name: "old_value",
    kind: "scalar",
    T: 9
    /* ScalarType.STRING */
  }
]);
var Field = _Field;

// index.ts
var rocketAddress = bytesFromHex("0xae78736Cd615f374D3085123A210448E74Fc6393");
var approvalTopic = bytesFromHex(
  "0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"
);
var transferTopic = bytesFromHex(
  "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
);
function popo() {
  console.log("Hello from popo!");
  const out = map_block(readInput());
  writeOutput(out);
}
function readInput() {
  const chunkSize = 1 * 1024 * 1024;
  const inputChunks = [];
  let totalBytes = 0;
  while (1) {
    const buffer = new Uint8Array(chunkSize);
    const fd = 0;
    const bytesRead = Javy.IO.readSync(fd, buffer);
    totalBytes += bytesRead;
    if (bytesRead === 0) {
      break;
    }
    inputChunks.push(buffer.subarray(0, bytesRead));
  }
  const { finalBuffer } = inputChunks.reduce(
    (context, chunk) => {
      context.finalBuffer.set(chunk, context.bufferOffset);
      context.bufferOffset += chunk.length;
      return context;
    },
    { bufferOffset: 0, finalBuffer: new Uint8Array(totalBytes) }
  );
  return finalBuffer;
}
function writeOutput(output) {
  const encodedOutput = new TextEncoder().encode(JSON.stringify(output));
  const buffer = new Uint8Array(encodedOutput);
  const fd = 1;
  Javy.IO.writeSync(fd, buffer);
}
function map_block(data) {
  const block = new Block();
  block.fromBinary(data);
  const changes = new DatabaseChanges();
  const blockNumberStr = block.header?.number.toString() ?? "";
  const blockTimestampStr = block.header?.timestamp?.seconds.toString() ?? "";
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
          const change = new TableChange();
          change.table = "Approval";
          change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` };
          change.operation = 1 /* CREATE */;
          change.ordinal = (0, import_bigInt.default)(0);
          change.fields = [
            new Field({ name: "timestamp", newValue: blockTimestampStr }),
            new Field({ name: "block_number", newValue: blockNumberStr }),
            new Field({ name: "log_index", newValue: log.index.toString() }),
            new Field({ name: "tx_hash", newValue: bytesToHex(trace.hash) }),
            new Field({ name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
            new Field({ name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
            new Field({ name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) })
          ];
          changes.tableChanges.push(change);
          return;
        }
        if (bytesEqual(log.topics[0], transferTopic)) {
          transferCount++;
          const change = new TableChange({});
          change.table = "Transfer";
          change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` };
          change.operation = 1 /* CREATE */;
          change.ordinal = (0, import_bigInt.default)(0);
          change.fields = [
            new Field({ name: "timestamp", newValue: blockTimestampStr }),
            new Field({ name: "block_number", newValue: blockNumberStr }),
            new Field({ name: "log_index", newValue: log.index.toString() }),
            new Field({ name: "tx_hash", newValue: bytesToHex(trace.hash) }),
            new Field({ name: "sender", newValue: bytesToHex(log.topics[1].slice(12)) }),
            new Field({ name: "receiver", newValue: bytesToHex(log.topics[2].slice(12)) }),
            new Field({ name: "value", newValue: bytesToHex(stripZeroBytes(log.data)) })
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
  for (let i = 0; i != input.length; i++) {
    if (input[i] != 0) {
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
  return String.fromCharCode.apply(null, chars);
}
function bytesFromHex(hex) {
  if (hex.match(/^0(x|X)/)) {
    hex = hex.slice(2);
  }
  if (hex.length % 2 !== 0) {
    hex = "0" + hex;
  }
  let i = 0;
  let bytes = new Uint8Array(hex.length / 2);
  for (let c = 0; c < hex.length; c += 2) {
    bytes[i] = parseInt(hex.slice(c, c + 2), 16);
    i++;
  }
  return bytes;
}
function bytesEqual(left, right) {
  if (left.length != right.length)
    return false;
  for (var i = 0; i != left.byteLength; i++) {
    if (left[i] != right[i])
      return false;
  }
  return true;
}
export {
  popo
};
