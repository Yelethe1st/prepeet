/**
 * The cross-journey state contract (WEB-04): the eleven states every data
 * surface supports, as shared components. Content rules live in README.md;
 * the required props are those rules, enforced.
 */
export {
  LoadingSurface,
  SkeletonBlock,
  SkeletonCircle,
  SkeletonText,
  type SkeletonTextWidth,
} from "./Skeleton";
export {
  ConnectionState,
  DegradedState,
  DelayedState,
  EmptyState,
  ErrorState,
  ExpiredState,
  ForbiddenState,
  InsufficientEvidenceState,
  PartialState,
  UnassessableState,
} from "./states";
export { SurfaceState } from "./SurfaceState";
