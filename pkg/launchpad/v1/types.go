// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import "time"

// Collection is the generic wrapper for paginated Launchpad API collections.
type Collection[T any] struct {
	TotalSize          int    `json:"total_size"`
	Start              int    `json:"start"`
	NextCollectionLink string `json:"next_collection_link,omitempty"`
	PrevCollectionLink string `json:"prev_collection_link,omitempty"`
	Entries            []T    `json:"entries"`
}

// Person is a Launchpad person or team.
type Person struct {
	Name                      string `json:"name"`
	DisplayName               string `json:"display_name"`
	SelfLink                  string `json:"self_link"`
	WebLink                   string `json:"web_link"`
	ResourceTypeLink          string `json:"resource_type_link"`
	HTTPEtag                  string `json:"http_etag"`
	IsTeam                    bool   `json:"is_team"`
	IsValid                   bool   `json:"is_valid"`
	Karma                     int    `json:"karma"`
	Description               string `json:"description"`
	DateCreated               *Time  `json:"date_created,omitempty"`
	TimeZone                  string `json:"time_zone"`
	Private                   bool   `json:"private"`
	Visibility                string `json:"visibility"`
	AccountStatus             string `json:"account_status"`
	HideEmailAddresses        bool   `json:"hide_email_addresses"`
	PreferredEmailAddressLink string `json:"preferred_email_address_link,omitempty"`
	IsProbationary            bool   `json:"is_probationary"`
	IsUbuntuCocSigner         bool   `json:"is_ubuntu_coc_signer"`
	ArchiveLink               string `json:"archive_link"`
	PPAsCollectionLink        string `json:"ppas_collection_link"`
	MembersCollectionLink     string `json:"members_collection_link"`
	TeamOwnerLink             string `json:"team_owner_link"`
	LogoLink                  string `json:"logo_link"`
	MugshotLink               string `json:"mugshot_link"`
}

// Project represents a Launchpad project (product).
type Project struct {
	Name                           string   `json:"name"`
	DisplayName                    string   `json:"display_name"`
	Title                          string   `json:"title"`
	Summary                        string   `json:"summary"`
	Description                    string   `json:"description"`
	SelfLink                       string   `json:"self_link"`
	WebLink                        string   `json:"web_link"`
	ResourceTypeLink               string   `json:"resource_type_link"`
	HTTPEtag                       string   `json:"http_etag"`
	Active                         bool     `json:"active"`
	DateCreated                    *Time    `json:"date_created,omitempty"`
	HomepageURL                    string   `json:"homepage_url"`
	DownloadURL                    string   `json:"download_url"`
	WikiURL                        string   `json:"wiki_url"`
	ScreenshotsURL                 string   `json:"screenshots_url"`
	ProgrammingLanguage            string   `json:"programming_language"`
	OwnerLink                      string   `json:"owner_link"`
	DriverLink                     string   `json:"driver_link"`
	RegistrantLink                 string   `json:"registrant_link"`
	BugSupervisorLink              string   `json:"bug_supervisor_link"`
	BugTrackerLink                 string   `json:"bug_tracker_link"`
	ProjectGroupLink               string   `json:"project_group_link"`
	DevelopmentFocusLink           string   `json:"development_focus_link"`
	TranslationFocusLink           string   `json:"translation_focus_link"`
	IconLink                       string   `json:"icon_link"`
	LogoLink                       string   `json:"logo_link"`
	BrandLink                      string   `json:"brand_link"`
	VCS                            string   `json:"vcs"`
	InformationType                string   `json:"information_type"`
	Private                        bool     `json:"private"`
	Licenses                       []string `json:"licenses"`
	LicenseInfo                    string   `json:"license_info"`
	OfficialBugTags                []string `json:"official_bug_tags"`
	OfficialBugs                   bool     `json:"official_bugs"`
	OfficialBlueprints             bool     `json:"official_blueprints"`
	OfficialCodehosting            bool     `json:"official_codehosting"`
	OfficialAnswers                bool     `json:"official_answers"`
	BugReportingGuidelines         string   `json:"bug_reporting_guidelines"`
	BugReportedAcknowledgement     string   `json:"bug_reported_acknowledgement"`
	RemoteProduct                  string   `json:"remote_product"`
	ActiveMilestonesCollectionLink string   `json:"active_milestones_collection_link"`
	AllMilestonesCollectionLink    string   `json:"all_milestones_collection_link"`
	SeriesCollectionLink           string   `json:"series_collection_link"`
	ReleasesCollectionLink         string   `json:"releases_collection_link"`
	RecipesCollectionLink          string   `json:"recipes_collection_link"`
	WebhooksCollectionLink         string   `json:"webhooks_collection_link"`
	CommercialSubscriptionLink     string   `json:"commercial_subscription_link"`
	CommercialSubscriptionIsDue    bool     `json:"commercial_subscription_is_due"`
	QualifiesForFreeHosting        bool     `json:"qualifies_for_free_hosting"`
}

// Bug represents a Launchpad bug.
type Bug struct {
	ID                                 int      `json:"id"`
	Title                              string   `json:"title"`
	Description                        string   `json:"description"`
	SelfLink                           string   `json:"self_link"`
	WebLink                            string   `json:"web_link"`
	ResourceTypeLink                   string   `json:"resource_type_link"`
	HTTPEtag                           string   `json:"http_etag"`
	OwnerLink                          string   `json:"owner_link"`
	Tags                               []string `json:"tags"`
	Heat                               int      `json:"heat"`
	Private                            bool     `json:"private"`
	SecurityRelated                    bool     `json:"security_related"`
	InformationType                    string   `json:"information_type"`
	Name                               string   `json:"name"`
	DateCreated                        *Time    `json:"date_created,omitempty"`
	DateLastUpdated                    *Time    `json:"date_last_updated,omitempty"`
	DateLastMessage                    *Time    `json:"date_last_message,omitempty"`
	MessageCount                       int      `json:"message_count"`
	NumberOfDuplicates                 int      `json:"number_of_duplicates"`
	UsersAffectedCount                 int      `json:"users_affected_count"`
	DuplicateOfLink                    string   `json:"duplicate_of_link"`
	LatestPatchUploaded                *Time    `json:"latest_patch_uploaded,omitempty"`
	LockStatus                         string   `json:"lock_status"`
	LockReason                         string   `json:"lock_reason"`
	BugTasksCollectionLink             string   `json:"bug_tasks_collection_link"`
	AttachmentsCollectionLink          string   `json:"attachments_collection_link"`
	ActivityCollectionLink             string   `json:"activity_collection_link"`
	MessagesCollectionLink             string   `json:"messages_collection_link"`
	SubscriptionsCollectionLink        string   `json:"subscriptions_collection_link"`
	LinkedBranchesCollectionLink       string   `json:"linked_branches_collection_link"`
	LinkedMergeProposalsCollectionLink string   `json:"linked_merge_proposals_collection_link"`
	DuplicatesCollectionLink           string   `json:"duplicates_collection_link"`
	CVEsCollectionLink                 string   `json:"cves_collection_link"`
	VulnerabilitiesCollectionLink      string   `json:"vulnerabilities_collection_link"`
	UsersAffectedCollectionLink        string   `json:"users_affected_collection_link"`
	UsersUnaffectedCollectionLink      string   `json:"users_unaffected_collection_link"`
}

// Message represents a comment in a Launchpad bug discussion.
type Message struct {
	SelfLink       string `json:"self_link"`
	WebLink        string `json:"web_link"`
	Content        string `json:"content"`
	OwnerLink      string `json:"owner_link"`
	Subject        string `json:"subject"`
	Visible        bool   `json:"visible"`
	DateCreated    *Time  `json:"date_created,omitempty"`
	DateLastEdited *Time  `json:"date_last_edited,omitempty"`
}

// BugTask represents a bug task (a bug needing fixing in a particular product or package).
type BugTask struct {
	SelfLink                   string `json:"self_link"`
	WebLink                    string `json:"web_link"`
	ResourceTypeLink           string `json:"resource_type_link"`
	HTTPEtag                   string `json:"http_etag"`
	BugLink                    string `json:"bug_link"`
	BugTargetDisplayName       string `json:"bug_target_display_name"`
	BugTargetName              string `json:"bug_target_name"`
	TargetLink                 string `json:"target_link"`
	Title                      string `json:"title"`
	Status                     string `json:"status"`
	Importance                 string `json:"importance"`
	AssigneeLink               string `json:"assignee_link"`
	OwnerLink                  string `json:"owner_link"`
	MilestoneLink              string `json:"milestone_link"`
	BugWatchLink               string `json:"bug_watch_link"`
	StatusExplanation          string `json:"status_explanation"`
	ImportanceExplanation      string `json:"importance_explanation"`
	IsComplete                 bool   `json:"is_complete"`
	DateCreated                *Time  `json:"date_created,omitempty"`
	DateAssigned               *Time  `json:"date_assigned,omitempty"`
	DateClosed                 *Time  `json:"date_closed,omitempty"`
	DateConfirmed              *Time  `json:"date_confirmed,omitempty"`
	DateFixCommitted           *Time  `json:"date_fix_committed,omitempty"`
	DateFixReleased            *Time  `json:"date_fix_released,omitempty"`
	DateInProgress             *Time  `json:"date_in_progress,omitempty"`
	DateIncomplete             *Time  `json:"date_incomplete,omitempty"`
	DateTriaged                *Time  `json:"date_triaged,omitempty"`
	DateLeftNew                *Time  `json:"date_left_new,omitempty"`
	DateLeftClosed             *Time  `json:"date_left_closed,omitempty"`
	RelatedTasksCollectionLink string `json:"related_tasks_collection_link"`
}

// GitRepository represents a Launchpad Git repository.
type GitRepository struct {
	ID                              int    `json:"id"`
	Name                            string `json:"name"`
	DisplayName                     string `json:"display_name"`
	Description                     string `json:"description"`
	SelfLink                        string `json:"self_link"`
	WebLink                         string `json:"web_link"`
	ResourceTypeLink                string `json:"resource_type_link"`
	HTTPEtag                        string `json:"http_etag"`
	GitIdentity                     string `json:"git_identity"`
	GitHTTPSURL                     string `json:"git_https_url"`
	GitSSHURL                       string `json:"git_ssh_url"`
	UniqueName                      string `json:"unique_name"`
	DefaultBranch                   string `json:"default_branch"`
	OwnerLink                       string `json:"owner_link"`
	RegistrantLink                  string `json:"registrant_link"`
	TargetLink                      string `json:"target_link"`
	ReviewerLink                    string `json:"reviewer_link"`
	OwnerDefault                    bool   `json:"owner_default"`
	TargetDefault                   bool   `json:"target_default"`
	Private                         bool   `json:"private"`
	InformationType                 string `json:"information_type"`
	RepositoryType                  string `json:"repository_type"`
	DateCreated                     *Time  `json:"date_created,omitempty"`
	DateLastModified                *Time  `json:"date_last_modified,omitempty"`
	DateLastScanned                 *Time  `json:"date_last_scanned,omitempty"`
	DateLastRepacked                *Time  `json:"date_last_repacked,omitempty"`
	LooseObjectCount                int    `json:"loose_object_count"`
	PackCount                       int    `json:"pack_count"`
	BranchesCollectionLink          string `json:"branches_collection_link"`
	RefsCollectionLink              string `json:"refs_collection_link"`
	LandingCandidatesCollectionLink string `json:"landing_candidates_collection_link"`
	LandingTargetsCollectionLink    string `json:"landing_targets_collection_link"`
	DependentLandingsCollectionLink string `json:"dependent_landings_collection_link"`
	SubscribersCollectionLink       string `json:"subscribers_collection_link"`
	SubscriptionsCollectionLink     string `json:"subscriptions_collection_link"`
	RecipesCollectionLink           string `json:"recipes_collection_link"`
	WebhooksCollectionLink          string `json:"webhooks_collection_link"`
	CodeImportLink                  string `json:"code_import_link"`
}

// GitRef represents a reference (branch/tag) in a Git repository.
type GitRef struct {
	Path                            string `json:"path"`
	CommitSHA1                      string `json:"commit_sha1"`
	SelfLink                        string `json:"self_link"`
	WebLink                         string `json:"web_link"`
	ResourceTypeLink                string `json:"resource_type_link"`
	HTTPEtag                        string `json:"http_etag"`
	RepositoryLink                  string `json:"repository_link"`
	LandingCandidatesCollectionLink string `json:"landing_candidates_collection_link"`
	LandingTargetsCollectionLink    string `json:"landing_targets_collection_link"`
	DependentLandingsCollectionLink string `json:"dependent_landings_collection_link"`
	RecipesCollectionLink           string `json:"recipes_collection_link"`
}

// MergeProposal represents a branch merge proposal (unified for Bazaar and Git).
type MergeProposal struct {
	SelfLink            string `json:"self_link"`
	WebLink             string `json:"web_link"`
	ResourceTypeLink    string `json:"resource_type_link"`
	HTTPEtag            string `json:"http_etag"`
	Description         string `json:"description"`
	CommitMessage       string `json:"commit_message"`
	QueueStatus         string `json:"queue_status"`
	Private             bool   `json:"private"`
	DateCreated         *Time  `json:"date_created,omitempty"`
	DateMerged          *Time  `json:"date_merged,omitempty"`
	DateReviewed        *Time  `json:"date_reviewed,omitempty"`
	DateReviewRequested *Time  `json:"date_review_requested,omitempty"`
	RegistrantLink      string `json:"registrant_link"`
	ReviewerLink        string `json:"reviewer_link"`
	MergeReporterLink   string `json:"merge_reporter_link"`
	Address             string `json:"address"`
	ReviewedRevID       string `json:"reviewed_revid"`
	MergedRevisionID    string `json:"merged_revision_id"`
	MergedRevno         *int   `json:"merged_revno,omitempty"`
	// Bazaar fields
	SourceBranchLink       string `json:"source_branch_link"`
	TargetBranchLink       string `json:"target_branch_link"`
	PrerequisiteBranchLink string `json:"prerequisite_branch_link"`
	// Git fields
	SourceGitRepositoryLink       string `json:"source_git_repository_link"`
	SourceGitPath                 string `json:"source_git_path"`
	TargetGitRepositoryLink       string `json:"target_git_repository_link"`
	TargetGitPath                 string `json:"target_git_path"`
	PrerequisiteGitRepositoryLink string `json:"prerequisite_git_repository_link"`
	PrerequisiteGitPath           string `json:"prerequisite_git_path"`
	// Related
	SupersededByLink           string `json:"superseded_by_link"`
	SupersedesLink             string `json:"supersedes_link"`
	PreviewDiffLink            string `json:"preview_diff_link"`
	AllCommentsCollectionLink  string `json:"all_comments_collection_link"`
	VotesCollectionLink        string `json:"votes_collection_link"`
	BugsCollectionLink         string `json:"bugs_collection_link"`
	PreviewDiffsCollectionLink string `json:"preview_diffs_collection_link"`
}

// CodeReviewComment links one merge proposal discussion entry to a message.
type CodeReviewComment struct {
	SelfLink       string `json:"self_link"`
	WebLink        string `json:"web_link"`
	Content        string `json:"content"`
	AuthorLink     string `json:"author_link"`
	DateCreated    *Time  `json:"date_created,omitempty"`
	DateLastEdited *Time  `json:"date_last_edited,omitempty"`
	Subject        string `json:"subject"`
}

// PreviewDiff is Launchpad's rendered merge proposal diff.
type PreviewDiff struct {
	SelfLink       string `json:"self_link"`
	DiffTextLink   string `json:"diff_text_link"`
	Diffstat       string `json:"diffstat"`
	DiffLinesCount int    `json:"diff_lines_count"`
}

// Archive represents a Launchpad archive (PPA or distribution archive).
type Archive struct {
	Name                              string `json:"name"`
	Displayname                       string `json:"displayname"`
	Description                       string `json:"description"`
	SelfLink                          string `json:"self_link"`
	WebLink                           string `json:"web_link"`
	ResourceTypeLink                  string `json:"resource_type_link"`
	HTTPEtag                          string `json:"http_etag"`
	Reference                         string `json:"reference"`
	OwnerLink                         string `json:"owner_link"`
	DistributionLink                  string `json:"distribution_link"`
	Private                           bool   `json:"private"`
	Status                            string `json:"status"`
	Publish                           bool   `json:"publish"`
	RequireVirtualized                bool   `json:"require_virtualized"`
	AuthorizedSize                    *int   `json:"authorized_size,omitempty"`
	ExternalDependencies              string `json:"external_dependencies"`
	SigningKeyFingerprint             string `json:"signing_key_fingerprint"`
	RelativeBuildScore                int    `json:"relative_build_score"`
	BuildDebugSymbols                 bool   `json:"build_debug_symbols"`
	PublishDebugSymbols               bool   `json:"publish_debug_symbols"`
	PermitObsoleteSeriesUploads       bool   `json:"permit_obsolete_series_uploads"`
	SuppressSubscriptionNotifications bool   `json:"suppress_subscription_notifications"`
	PublishingMethod                  string `json:"publishing_method"`
	RepositoryFormat                  string `json:"repository_format"`
	DependenciesCollectionLink        string `json:"dependencies_collection_link"`
	ProcessorsCollectionLink          string `json:"processors_collection_link"`
	WebhooksCollectionLink            string `json:"webhooks_collection_link"`
}

// SourcePublishing represents a source package publishing history entry.
type SourcePublishing struct {
	SelfLink              string `json:"self_link"`
	ResourceTypeLink      string `json:"resource_type_link"`
	HTTPEtag              string `json:"http_etag"`
	SourcePackageName     string `json:"source_package_name"`
	SourcePackageVersion  string `json:"source_package_version"`
	ComponentName         string `json:"component_name"`
	SectionName           string `json:"section_name"`
	Status                string `json:"status"`
	Pocket                string `json:"pocket"`
	DisplayName           string `json:"display_name"`
	ArchiveLink           string `json:"archive_link"`
	DistroSeriesLink      string `json:"distro_series_link"`
	CreatorLink           string `json:"creator_link"`
	PackageCreatorLink    string `json:"package_creator_link"`
	PackageMaintainerLink string `json:"package_maintainer_link"`
	PackageSignerLink     string `json:"package_signer_link"`
	SponsorLink           string `json:"sponsor_link"`
	RemovedByLink         string `json:"removed_by_link"`
	CopiedFromArchiveLink string `json:"copied_from_archive_link"`
	PackageUploadLink     string `json:"packageupload_link"`
	RemovalComment        string `json:"removal_comment"`
	DateCreated           *Time  `json:"date_created,omitempty"`
	DatePublished         *Time  `json:"date_published,omitempty"`
	DateSuperseded        *Time  `json:"date_superseded,omitempty"`
	DateRemoved           *Time  `json:"date_removed,omitempty"`
	DateMadePending       *Time  `json:"date_made_pending,omitempty"`
	ScheduledDeletionDate *Time  `json:"scheduled_deletion_date,omitempty"`
}

// PackageUpload is a pending Ubuntu archive queue item.
type PackageUpload struct {
	SelfLink       string `json:"self_link"`
	PackageName    string `json:"package_name"`
	PackageVersion string `json:"package_version"`
	Status         string `json:"status"`
	Pocket         string `json:"pocket"`
	ChangesFileURL string `json:"changes_file_url"`
	DistroSeries   string `json:"distroseries_link"`
}

// BinaryPublishing represents a binary package publishing history entry.
type BinaryPublishing struct {
	SelfLink               string `json:"self_link"`
	ResourceTypeLink       string `json:"resource_type_link"`
	HTTPEtag               string `json:"http_etag"`
	BinaryPackageName      string `json:"binary_package_name"`
	BinaryPackageVersion   string `json:"binary_package_version"`
	ComponentName          string `json:"component_name"`
	SectionName            string `json:"section_name"`
	PriorityName           string `json:"priority_name"`
	Status                 string `json:"status"`
	Pocket                 string `json:"pocket"`
	DisplayName            string `json:"display_name"`
	ArchitectureSpecific   bool   `json:"architecture_specific"`
	IsDebug                bool   `json:"is_debug"`
	ArchiveLink            string `json:"archive_link"`
	DistroArchSeriesLink   string `json:"distro_arch_series_link"`
	BuildLink              string `json:"build_link"`
	CreatorLink            string `json:"creator_link"`
	RemovedByLink          string `json:"removed_by_link"`
	CopiedFromArchiveLink  string `json:"copied_from_archive_link"`
	RemovalComment         string `json:"removal_comment"`
	SourcePackageName      string `json:"source_package_name"`
	SourcePackageVersion   string `json:"source_package_version"`
	PhasedUpdatePercentage *int   `json:"phased_update_percentage,omitempty"`
	DateCreated            *Time  `json:"date_created,omitempty"`
	DatePublished          *Time  `json:"date_published,omitempty"`
	DateSuperseded         *Time  `json:"date_superseded,omitempty"`
	DateRemoved            *Time  `json:"date_removed,omitempty"`
	DateMadePending        *Time  `json:"date_made_pending,omitempty"`
	ScheduledDeletionDate  *Time  `json:"scheduled_deletion_date,omitempty"`
}

// RockRecipe represents a buildable rock recipe on Launchpad.
type RockRecipe struct {
	Name                               string            `json:"name"`
	Description                        string            `json:"description"`
	SelfLink                           string            `json:"self_link"`
	WebLink                            string            `json:"web_link"`
	ResourceTypeLink                   string            `json:"resource_type_link"`
	HTTPEtag                           string            `json:"http_etag"`
	OwnerLink                          string            `json:"owner_link"`
	ProjectLink                        string            `json:"project_link"`
	RegistrantLink                     string            `json:"registrant_link"`
	GitRepositoryLink                  string            `json:"git_repository_link"`
	GitRefLink                         string            `json:"git_ref_link"`
	GitPath                            string            `json:"git_path"`
	GitRepositoryURL                   string            `json:"git_repository_url"`
	BuildPath                          string            `json:"build_path"`
	InformationType                    string            `json:"information_type"`
	Private                            bool              `json:"private"`
	RequireVirtualized                 bool              `json:"require_virtualized"`
	AutoBuild                          bool              `json:"auto_build"`
	AutoBuildChannels                  map[string]string `json:"auto_build_channels"`
	IsStale                            bool              `json:"is_stale"`
	StoreName                          string            `json:"store_name"`
	StoreUpload                        bool              `json:"store_upload"`
	StoreChannels                      []string          `json:"store_channels"`
	CanUploadToStore                   bool              `json:"can_upload_to_store"`
	UseFetchService                    bool              `json:"use_fetch_service"`
	FetchServicePolicy                 string            `json:"fetch_service_policy"`
	DateCreated                        *Time             `json:"date_created,omitempty"`
	DateLastModified                   *Time             `json:"date_last_modified,omitempty"`
	BuildsCollectionLink               string            `json:"builds_collection_link"`
	CompletedBuildsCollectionLink      string            `json:"completed_builds_collection_link"`
	PendingBuildsCollectionLink        string            `json:"pending_builds_collection_link"`
	FailedBuildRequestsCollectionLink  string            `json:"failed_build_requests_collection_link"`
	PendingBuildRequestsCollectionLink string            `json:"pending_build_requests_collection_link"`
}

// RockRecipeBuild represents a build record for a rock recipe.
type RockRecipeBuild struct {
	SelfLink             string            `json:"self_link"`
	WebLink              string            `json:"web_link"`
	ResourceTypeLink     string            `json:"resource_type_link"`
	HTTPEtag             string            `json:"http_etag"`
	Title                string            `json:"title"`
	RecipeLink           string            `json:"recipe_link"`
	RequesterLink        string            `json:"requester_link"`
	BuilderLink          string            `json:"builder_link"`
	ArchiveLink          string            `json:"archive_link"`
	ArchTag              string            `json:"arch_tag"`
	BuildState           string            `json:"buildstate"`
	BuildLogURL          string            `json:"build_log_url"`
	BuildMetadataURL     string            `json:"build_metadata_url"`
	UploadLogURL         string            `json:"upload_log_url"`
	RevisionID           string            `json:"revision_id"`
	Score                *int              `json:"score,omitempty"`
	CanBeCancelled       bool              `json:"can_be_cancelled"`
	CanBeRescored        bool              `json:"can_be_rescored"`
	CanBeRetried         bool              `json:"can_be_retried"`
	Channels             map[string]string `json:"channels"`
	Dependencies         string            `json:"dependencies"`
	Pocket               string            `json:"pocket"`
	DistributionLink     string            `json:"distribution_link"`
	DistroSeriesLink     string            `json:"distro_series_link"`
	DistroArchSeriesLink string            `json:"distro_arch_series_link"`
	Duration             string            `json:"duration"`
	DateCreated          *Time             `json:"datecreated,omitempty"`
	DateStarted          *Time             `json:"date_started,omitempty"`
	DateBuilt            *Time             `json:"datebuilt,omitempty"`
	DateFirstDispatched  *Time             `json:"date_first_dispatched,omitempty"`
}

// CharmRecipe represents a buildable charm recipe on Launchpad.
type CharmRecipe struct {
	Name                               string            `json:"name"`
	Description                        string            `json:"description"`
	SelfLink                           string            `json:"self_link"`
	WebLink                            string            `json:"web_link"`
	ResourceTypeLink                   string            `json:"resource_type_link"`
	HTTPEtag                           string            `json:"http_etag"`
	OwnerLink                          string            `json:"owner_link"`
	ProjectLink                        string            `json:"project_link"`
	RegistrantLink                     string            `json:"registrant_link"`
	GitRepositoryLink                  string            `json:"git_repository_link"`
	GitRefLink                         string            `json:"git_ref_link"`
	GitPath                            string            `json:"git_path"`
	GitRepositoryURL                   string            `json:"git_repository_url"`
	BuildPath                          string            `json:"build_path"`
	InformationType                    string            `json:"information_type"`
	Private                            bool              `json:"private"`
	RequireVirtualized                 bool              `json:"require_virtualized"`
	AutoBuild                          bool              `json:"auto_build"`
	AutoBuildChannels                  map[string]string `json:"auto_build_channels"`
	IsStale                            bool              `json:"is_stale"`
	StoreName                          string            `json:"store_name"`
	StoreUpload                        bool              `json:"store_upload"`
	StoreChannels                      []string          `json:"store_channels"`
	CanUploadToStore                   bool              `json:"can_upload_to_store"`
	UseFetchService                    bool              `json:"use_fetch_service"`
	FetchServicePolicy                 string            `json:"fetch_service_policy"`
	DateCreated                        *Time             `json:"date_created,omitempty"`
	DateLastModified                   *Time             `json:"date_last_modified,omitempty"`
	BuildsCollectionLink               string            `json:"builds_collection_link"`
	CompletedBuildsCollectionLink      string            `json:"completed_builds_collection_link"`
	PendingBuildsCollectionLink        string            `json:"pending_builds_collection_link"`
	FailedBuildRequestsCollectionLink  string            `json:"failed_build_requests_collection_link"`
	PendingBuildRequestsCollectionLink string            `json:"pending_build_requests_collection_link"`
	WebhooksCollectionLink             string            `json:"webhooks_collection_link"`
}

// CharmRecipeBuild represents a build record for a charm recipe.
type CharmRecipeBuild struct {
	SelfLink             string            `json:"self_link"`
	WebLink              string            `json:"web_link"`
	ResourceTypeLink     string            `json:"resource_type_link"`
	HTTPEtag             string            `json:"http_etag"`
	Title                string            `json:"title"`
	RecipeLink           string            `json:"recipe_link"`
	RequesterLink        string            `json:"requester_link"`
	BuilderLink          string            `json:"builder_link"`
	ArchiveLink          string            `json:"archive_link"`
	ArchTag              string            `json:"arch_tag"`
	BuildState           string            `json:"buildstate"`
	BuildLogURL          string            `json:"build_log_url"`
	BuildMetadataURL     string            `json:"build_metadata_url"`
	UploadLogURL         string            `json:"upload_log_url"`
	RevisionID           string            `json:"revision_id"`
	CraftPlatform        string            `json:"craft_platform"`
	Score                *int              `json:"score,omitempty"`
	CanBeCancelled       bool              `json:"can_be_cancelled"`
	CanBeRescored        bool              `json:"can_be_rescored"`
	CanBeRetried         bool              `json:"can_be_retried"`
	Channels             map[string]string `json:"channels"`
	Dependencies         string            `json:"dependencies"`
	Pocket               string            `json:"pocket"`
	DistributionLink     string            `json:"distribution_link"`
	DistroSeriesLink     string            `json:"distro_series_link"`
	DistroArchSeriesLink string            `json:"distro_arch_series_link"`
	Duration             string            `json:"duration"`
	StoreUploadStatus    string            `json:"store_upload_status"`
	StoreUploadErrorMsg  string            `json:"store_upload_error_message"`
	StoreUploadRevision  *int              `json:"store_upload_revision,omitempty"`
	DateCreated          *Time             `json:"datecreated,omitempty"`
	DateStarted          *Time             `json:"date_started,omitempty"`
	DateBuilt            *Time             `json:"datebuilt,omitempty"`
	DateFirstDispatched  *Time             `json:"date_first_dispatched,omitempty"`
}

// Snap represents a buildable snap package on Launchpad.
type Snap struct {
	Name                               string            `json:"name"`
	Description                        string            `json:"description"`
	SelfLink                           string            `json:"self_link"`
	WebLink                            string            `json:"web_link"`
	ResourceTypeLink                   string            `json:"resource_type_link"`
	HTTPEtag                           string            `json:"http_etag"`
	OwnerLink                          string            `json:"owner_link"`
	RegistrantLink                     string            `json:"registrant_link"`
	GitRepositoryLink                  string            `json:"git_repository_link"`
	GitRefLink                         string            `json:"git_ref_link"`
	GitPath                            string            `json:"git_path"`
	GitRepositoryURL                   string            `json:"git_repository_url"`
	BranchLink                         string            `json:"branch_link"`
	InformationType                    string            `json:"information_type"`
	Private                            bool              `json:"private"`
	RequireVirtualized                 bool              `json:"require_virtualized"`
	AllowInternet                      bool              `json:"allow_internet"`
	BuildSourceTarball                 bool              `json:"build_source_tarball"`
	AutoBuild                          bool              `json:"auto_build"`
	AutoBuildArchiveLink               string            `json:"auto_build_archive_link"`
	AutoBuildChannels                  map[string]string `json:"auto_build_channels"`
	AutoBuildPocket                    string            `json:"auto_build_pocket"`
	IsStale                            bool              `json:"is_stale"`
	StoreName                          string            `json:"store_name"`
	StoreUpload                        bool              `json:"store_upload"`
	StoreChannels                      []string          `json:"store_channels"`
	StoreSeriesLink                    string            `json:"store_series_link"`
	CanUploadToStore                   bool              `json:"can_upload_to_store"`
	ProEnable                          bool              `json:"pro_enable"`
	UseFetchService                    bool              `json:"use_fetch_service"`
	FetchServicePolicy                 string            `json:"fetch_service_policy"`
	DistroSeriesLink                   string            `json:"distro_series_link"`
	DateCreated                        *Time             `json:"date_created,omitempty"`
	DateLastModified                   *Time             `json:"date_last_modified,omitempty"`
	BuildsCollectionLink               string            `json:"builds_collection_link"`
	CompletedBuildsCollectionLink      string            `json:"completed_builds_collection_link"`
	PendingBuildsCollectionLink        string            `json:"pending_builds_collection_link"`
	FailedBuildRequestsCollectionLink  string            `json:"failed_build_requests_collection_link"`
	PendingBuildRequestsCollectionLink string            `json:"pending_build_requests_collection_link"`
	ProcessorsCollectionLink           string            `json:"processors_collection_link"`
	WebhooksCollectionLink             string            `json:"webhooks_collection_link"`
}

// SnapBuild represents a build record for a snap package.
type SnapBuild struct {
	SelfLink             string            `json:"self_link"`
	WebLink              string            `json:"web_link"`
	ResourceTypeLink     string            `json:"resource_type_link"`
	HTTPEtag             string            `json:"http_etag"`
	Title                string            `json:"title"`
	SnapLink             string            `json:"snap_link"`
	RequesterLink        string            `json:"requester_link"`
	BuilderLink          string            `json:"builder_link"`
	ArchiveLink          string            `json:"archive_link"`
	ArchTag              string            `json:"arch_tag"`
	BuildState           string            `json:"buildstate"`
	BuildLogURL          string            `json:"build_log_url"`
	BuildMetadataURL     string            `json:"build_metadata_url"`
	UploadLogURL         string            `json:"upload_log_url"`
	RevisionID           string            `json:"revision_id"`
	CraftPlatform        string            `json:"craft_platform"`
	Score                *int              `json:"score,omitempty"`
	CanBeCancelled       bool              `json:"can_be_cancelled"`
	CanBeRescored        bool              `json:"can_be_rescored"`
	CanBeRetried         bool              `json:"can_be_retried"`
	Channels             map[string]string `json:"channels"`
	Dependencies         string            `json:"dependencies"`
	Pocket               string            `json:"pocket"`
	DistributionLink     string            `json:"distribution_link"`
	DistroSeriesLink     string            `json:"distro_series_link"`
	DistroArchSeriesLink string            `json:"distro_arch_series_link"`
	SnapBaseLink         string            `json:"snap_base_link"`
	BuildRequestLink     string            `json:"build_request_link"`
	Duration             string            `json:"duration"`
	TargetArchitectures  []string          `json:"target_architectures"`
	StoreUploadStatus    string            `json:"store_upload_status"`
	StoreUploadErrorMsg  string            `json:"store_upload_error_message"`
	StoreUploadErrorMsgs []StoreUploadErr  `json:"store_upload_error_messages"`
	StoreUploadRevision  *int              `json:"store_upload_revision,omitempty"`
	StoreUploadURL       string            `json:"store_upload_url"`
	DateCreated          *Time             `json:"datecreated,omitempty"`
	DateStarted          *Time             `json:"date_started,omitempty"`
	DateBuilt            *Time             `json:"datebuilt,omitempty"`
	DateFirstDispatched  *Time             `json:"date_first_dispatched,omitempty"`
}

// StoreUploadErr represents an error message from a store upload attempt.
type StoreUploadErr struct {
	Message string `json:"message"`
	Link    string `json:"link"`
}

// BuildCounters holds the result of archive.getBuildCounters().
type BuildCounters struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Failed     int `json:"failed"`
	Succeeded  int `json:"succeeded"`
	Superseded int `json:"superseded"`
}

// BuildRequest represents a recipe build request (shared across rock/charm/snap).
type BuildRequest struct {
	SelfLink             string `json:"self_link"`
	WebLink              string `json:"web_link"`
	ResourceTypeLink     string `json:"resource_type_link"`
	HTTPEtag             string `json:"http_etag"`
	Status               string `json:"status"`
	ErrorMessage         string `json:"error_message"`
	DateRequested        *Time  `json:"date_requested,omitempty"`
	DateFinished         *Time  `json:"date_finished,omitempty"`
	BuildsCollectionLink string `json:"builds_collection_link"`
}

// Time wraps time.Time with Launchpad's JSON date format.
type Time struct {
	time.Time
}

// UnmarshalJSON parses Launchpad date strings (ISO 8601 / RFC 3339).
func (t *Time) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		return nil
	}
	// Strip quotes
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try without timezone
		parsed, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return err
		}
	}
	t.Time = parsed
	return nil
}

// MarshalJSON formats as RFC3339.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.Format(time.RFC3339) + `"`), nil
}
