package io.killedkenny.crossfuzz;

import java.lang.instrument.ClassFileTransformer;
import java.security.ProtectionDomain;
import java.util.HashSet;
import java.util.Set;
import org.objectweb.asm.ClassReader;
import org.objectweb.asm.ClassWriter;
import org.objectweb.asm.Opcodes;
import org.objectweb.asm.tree.AbstractInsnNode;
import org.objectweb.asm.tree.ClassNode;
import org.objectweb.asm.tree.FrameNode;
import org.objectweb.asm.tree.LineNumberNode;
import org.objectweb.asm.tree.InsnList;
import org.objectweb.asm.tree.JumpInsnNode;
import org.objectweb.asm.tree.LabelNode;
import org.objectweb.asm.tree.LdcInsnNode;
import org.objectweb.asm.tree.LookupSwitchInsnNode;
import org.objectweb.asm.tree.MethodInsnNode;
import org.objectweb.asm.tree.MethodNode;
import org.objectweb.asm.tree.TableSwitchInsnNode;
import org.objectweb.asm.tree.TryCatchBlockNode;

public class CoverageTransformer implements ClassFileTransformer {

    @Override
    public byte[] transform(ClassLoader loader, String className,
            Class<?> classBeingRedefined, ProtectionDomain pd, byte[] buf) {
        if (className == null) return null;
        // Bootstrap-loaded classes (java.*, javax.*, sun.*, jdk.*, etc.):
        // CoverageRuntime.hit() uses ByteBuffer internally — instrumenting those
        // same classes would recurse infinitely.
        if (loader == null) return null;
        // Never instrument the harness runtime or its ASM dependency.
        if (className.startsWith("io/killedkenny/crossfuzz/")) return null;
        if (className.startsWith("org/objectweb/asm/")) return null;
        try {
            ClassReader cr = new ClassReader(buf);
            ClassNode cn = new ClassNode();
            // EXPAND_FRAMES: parse and expand all StackMapTable entries into
            // FrameNodes in the instruction list. Combined with COMPUTE_MAXS
            // below this avoids loading any classes during instrumentation
            // (COMPUTE_FRAMES triggers getCommonSuperClass → Class.forName →
            // re-entrant loadClass while the class is mid-definition → LinkageError).
            cr.accept(cn, ClassReader.EXPAND_FRAMES);
            for (MethodNode mn : cn.methods) {
                instrumentMethod(className, mn);
            }
            // COMPUTE_MAXS: only recalculate max stack/locals. Our probes have
            // zero net stack effect (LDC int + INVOKESTATIC void), so existing
            // StackMapTable frames remain valid — no class loading required.
            ClassWriter cw = new ClassWriter(ClassWriter.COMPUTE_MAXS);
            cn.accept(cw);
            return cw.toByteArray();
        } catch (Throwable t) {
            return null;
        }
    }

    private void instrumentMethod(String cls, MethodNode mn) {
        // Abstract and native methods have no body. Inserting any instruction
        // would give them a Code attribute, which JVMS §4.7.3 forbids — the JVM
        // then rejects the class with ClassFormatError at defineClass time (after
        // transform() has already returned, so the catch in transform() cannot
        // recover from it). Skip these methods entirely.
        if ((mn.access & (Opcodes.ACC_ABSTRACT | Opcodes.ACC_NATIVE)) != 0) {
            return;
        }

        // Collect branch target labels (basic block entries)
        Set<LabelNode> targets = new HashSet<>();
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof JumpInsnNode) {
                targets.add(((JumpInsnNode) n).label);
            } else if (n instanceof TableSwitchInsnNode) {
                TableSwitchInsnNode ts = (TableSwitchInsnNode) n;
                targets.add(ts.dflt);
                ts.labels.forEach(targets::add);
            } else if (n instanceof LookupSwitchInsnNode) {
                LookupSwitchInsnNode ls = (LookupSwitchInsnNode) n;
                targets.add(ls.dflt);
                ls.labels.forEach(targets::add);
            }
        }
        for (TryCatchBlockNode tcb : mn.tryCatchBlocks) {
            targets.add(tcb.handler);
        }

        // Inject at method entry (before first instruction)
        mn.instructions.insert(makeHit(cls, mn.name, 0));

        // Inject after each branch-target label (and any pseudo-nodes that follow it).
        // With EXPAND_FRAMES, a label may be followed by a LineNumberNode, a FrameNode,
        // or both in either order, before the first real instruction. The StackMapTable
        // entry must remain at the label's effective bytecode offset, so the probe must
        // come after all of these pseudo-nodes. (Exception handler labels compiled with
        // debug info have LineNumberNode → FrameNode between the label and the handler
        // body; checking only for FrameNode misses the LineNumberNode and shifts the
        // frame past the probe, producing VerifyError: Expecting a stackmap frame at
        // branch target N.)
        int blockId = 1;
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof LabelNode && targets.contains(n)) {
                AbstractInsnNode insertAfter = n;
                while (insertAfter.getNext() instanceof FrameNode
                        || insertAfter.getNext() instanceof LineNumberNode) {
                    insertAfter = insertAfter.getNext();
                }
                mn.instructions.insert(insertAfter, makeHit(cls, mn.name, blockId++));
            }
        }
    }

    private InsnList makeHit(String cls, String method, int blockId) {
        int idx = (cls + "_" + method + "_" + blockId).hashCode() & 0xFFFF;
        InsnList l = new InsnList();
        l.add(new LdcInsnNode(idx));
        l.add(new MethodInsnNode(Opcodes.INVOKESTATIC,
            "io/killedkenny/crossfuzz/CoverageRuntime", "hit", "(I)V", false));
        return l;
    }
}
