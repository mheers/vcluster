package translate

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	NamespaceLabel  = "vcluster.loft.sh/namespace"
	MarkerLabel     = "vcluster.loft.sh/managed-by"
	LabelPrefix     = "vcluster.loft.sh/label"
	ControllerLabel = "vcluster.loft.sh/controlled-by"
	Suffix          = "suffix"

	ManagedAnnotationsAnnotation = "vcluster.loft.sh/managed-annotations"
	ManagedLabelsAnnotation      = "vcluster.loft.sh/managed-labels"
)

var Owner client.Object

func GetOwnerReference(object client.Object) []metav1.OwnerReference {
	if Owner == nil || Owner.GetName() == "" || Owner.GetUID() == "" {
		return nil
	}

	typeAccessor, err := meta.TypeAccessor(Owner)
	if err != nil || typeAccessor.GetAPIVersion() == "" || typeAccessor.GetKind() == "" {
		return nil
	}

	isController := false
	if object != nil {
		ctrl := metav1.GetControllerOf(object)
		isController = ctrl != nil
	}
	return []metav1.OwnerReference{
		{
			APIVersion: typeAccessor.GetAPIVersion(),
			Kind:       typeAccessor.GetKind(),
			Name:       Owner.GetName(),
			UID:        Owner.GetUID(),
			Controller: &isController,
		},
	}
}

func SafeConcatName(name ...string) string {
	fullPath := strings.Join(name, "-")
	if len(fullPath) > 63 {
		digest := sha256.Sum256([]byte(fullPath))
		return strings.ReplaceAll(fullPath[0:52]+"-"+hex.EncodeToString(digest[0:])[0:10], ".-", "-")
	}
	return fullPath
}

func UniqueSlice(stringSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range stringSlice {
		if entry == "" {
			continue
		}
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func Split(s, sep string) (string, string) {
	parts := strings.SplitN(s, sep, 2)
	return strings.TrimSpace(parts[0]), strings.TrimSpace(safeIndex(parts, 1))
}

func safeIndex(parts []string, idx int) string {
	if len(parts) <= idx {
		return ""
	}
	return parts[idx]
}

func exists(a []string, k string) bool {
	for _, i := range a {
		if i == k {
			return true
		}
	}

	return false
}

// ResetObjectMetadata resets the objects metadata except name, namespace and annotations
func ResetObjectMetadata(obj metav1.Object) {
	obj.SetGenerateName("")
	obj.SetSelfLink("")
	obj.SetUID("")
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetDeletionTimestamp(nil)
	obj.SetDeletionGracePeriodSeconds(nil)
	obj.SetOwnerReferences(nil)
	obj.SetFinalizers(nil)
	obj.SetManagedFields(nil)
}

func ApplyMetadata(fromAnnotations map[string]string, toAnnotations map[string]string, fromLabels map[string]string, toLabels map[string]string, excludeAnnotations ...string) (labels map[string]string, annotations map[string]string) {
	mergedAnnotations := applyAnnotations(fromAnnotations, toAnnotations, excludeAnnotations...)
	return applyLabels(fromLabels, toLabels, mergedAnnotations)
}

func applyAnnotations(fromAnnotations map[string]string, toAnnotations map[string]string, excludeAnnotations ...string) map[string]string {
	if toAnnotations == nil {
		toAnnotations = map[string]string{}
	}

	excludedKeys := []string{ManagedAnnotationsAnnotation, ManagedLabelsAnnotation}
	excludedKeys = append(excludedKeys, excludeAnnotations...)
	mergedAnnotations, managedKeys := applyMaps(fromAnnotations, toAnnotations, ApplyMapsOptions{
		ManagedKeys: strings.Split(toAnnotations[ManagedAnnotationsAnnotation], "\n"),
		ExcludeKeys: excludedKeys,
	})
	if managedKeys == "" {
		delete(mergedAnnotations, ManagedAnnotationsAnnotation)
	} else {
		mergedAnnotations[ManagedAnnotationsAnnotation] = managedKeys
	}

	return mergedAnnotations
}

func applyLabels(fromLabels map[string]string, toLabels map[string]string, toAnnotations map[string]string) (labels map[string]string, annotations map[string]string) {
	if toAnnotations == nil {
		toAnnotations = map[string]string{}
	}

	mergedLabels, managedKeys := applyMaps(fromLabels, toLabels, ApplyMapsOptions{
		ManagedKeys: strings.Split(toAnnotations[ManagedLabelsAnnotation], "\n"),
		ExcludeKeys: []string{ManagedAnnotationsAnnotation, ManagedLabelsAnnotation},
	})
	mergedAnnotations := map[string]string{}
	for k, v := range toAnnotations {
		mergedAnnotations[k] = v
	}
	if managedKeys == "" {
		delete(mergedAnnotations, ManagedLabelsAnnotation)
	} else {
		mergedAnnotations[ManagedLabelsAnnotation] = managedKeys
	}

	return mergedLabels, mergedAnnotations
}

type ApplyMapsOptions struct {
	ManagedKeys []string
	ExcludeKeys []string
}

func applyMaps(fromMap map[string]string, toMap map[string]string, opts ApplyMapsOptions) (map[string]string, string) {
	retMap := map[string]string{}
	managedKeys := []string{}
	for k, v := range fromMap {
		if exists(opts.ExcludeKeys, k) {
			continue
		}

		retMap[k] = v
		managedKeys = append(managedKeys, k)
	}

	for key, value := range toMap {
		if exists(opts.ExcludeKeys, key) {
			if value != "" {
				retMap[key] = value
			}
			continue
		} else if exists(managedKeys, key) || exists(opts.ManagedKeys, key) {
			continue
		}

		retMap[key] = value
	}

	sort.Strings(managedKeys)
	managedKeysStr := strings.Join(managedKeys, "\n")
	return retMap, managedKeysStr
}
