// Pure presentation geometry. Widths are preferences, never process rules.
(function(root) {
 "use strict";
 const clamp=(value,min,max)=>Math.min(max,Math.max(min,value));
 function preferences(value={}) {
  return {rail:clamp(Number(value.rail)||264,190,420),ratio:clamp(Number(value.ratio)||.48,.28,.68),collapsed:value.collapsed===true};
 }
 function inspector(width,ratio) {
  const max=Math.max(300,width-300);
  return Math.round(clamp(width*ratio,300,max));
 }
 const api={preferences,inspector,clamp};
 if(typeof module!=="undefined" && module.exports)module.exports=api;
 else root.WorkspaceColumns=api;
})(typeof window!=="undefined"?window:globalThis);
