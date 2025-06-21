#!/usr/bin/env fish

function psurf
    # TODO: set bin path here
    set app_path "/home/bleart/code/pathsurfer/bin/pathsurfer"
    
    if not test -x "$app_path"
        echo "error: pathsurfer not found or isn't executable at $app_path"
        return 1
    end

    set target_dir ("$app_path")

    if test -n "$target_dir"; and test -d "$target_dir"
        cd "$target_dir"
    end
end

